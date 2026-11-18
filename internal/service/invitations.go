package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"rosemary-virsree/internal/model"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/store"
)

func (s *Service) CreateUploadInvitation(ctx context.Context, b model.Bucket, name string, maxFiles, expires int64, overwrite bool) (store.UploadInvitation, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 160 || !utf8.ValidString(name) || maxFiles < 0 || maxFiles > 1000000 || expires < 60 || expires > 604800 {
		return store.UploadInvitation{}, "", errors.New("invalid invitation name, file limit or expires_in")
	}
	raw := secretbox.Random("rvs_au_", 24)
	i := store.UploadInvitation{ID: secretbox.Random("inv_", 12), BucketID: b.ID, Name: name, TokenHash: secretbox.Hash(raw), MaxFiles: maxFiles, AllowOverwrite: overwrite, ExpiresAt: time.Now().UTC().Add(time.Duration(expires) * time.Second), CreatedAt: time.Now().UTC()}
	err := s.DB.CreateUploadInvitation(ctx, i)
	if err == nil {
		s.DB.Audit(ctx, "upload-invitation.created", i.ID, b.Slug)
	}
	return i, raw, err
}
func (s *Service) AuthenticateUploadInvitation(ctx context.Context, raw string) (store.UploadInvitation, Credential, error) {
	if !strings.HasPrefix(raw, "rvs_au_") {
		return store.UploadInvitation{}, Credential{}, store.ErrInvitationInactive
	}
	i, err := s.DB.UploadInvitationByHash(ctx, secretbox.Hash(raw))
	if err != nil || i.Revoked || !time.Now().Before(i.ExpiresAt) {
		return i, Credential{}, store.ErrInvitationInactive
	}
	slug, err := s.DB.InvitationBucketSlug(ctx, i.BucketID)
	if err != nil {
		return i, Credential{}, store.ErrInvitationInactive
	}
	b, err := s.DB.GetBucket(ctx, slug)
	if err != nil || b.Deleting {
		return i, Credential{}, store.ErrInvitationInactive
	}
	return i, Credential{Bucket: b}, nil
}
func (s *Service) BeginInvitedUpload(ctx context.Context, i store.UploadInvitation, c Credential, key, contentType string, size int64) (model.Object, string, int64, error) {
	remaining := int64(time.Until(i.ExpiresAt) / time.Second)
	if remaining < 1 {
		return model.Object{}, "", 0, store.ErrInvitationInactive
	}
	if remaining > 900 {
		remaining = 900
	}
	o, u, err := s.beginUpload(ctx, c, key, contentType, size, remaining, i.ID)
	return o, u, remaining, err
}
func (s *Service) CommitInvitedUpload(ctx context.Context, i store.UploadInvitation, c Credential, id, key string) (model.Object, string, error) {
	receipt, err := s.DB.RequireLiveInvitationUpload(ctx, i.ID, id, key)
	if err != nil {
		return model.Object{}, "", err
	}
	var o model.Object
	if receipt.Status == "committed" {
		o, err = s.DB.GetObject(ctx, c.Bucket.ID, key)
		if err != nil || o.ID != receipt.ObjectID {
			return model.Object{}, "", store.ErrInvitationOwnership
		}
	} else {
		o, err = s.CommitUpload(ctx, c, id, key)
		if err != nil {
			// A parallel retry may have committed between the ownership check and lock.
			receipt, retryErr := s.DB.RequireLiveInvitationUpload(ctx, i.ID, id, key)
			if retryErr != nil || receipt.Status != "committed" {
				return model.Object{}, "", err
			}
			o, err = s.DB.GetObject(ctx, c.Bucket.ID, key)
		}
		if err != nil {
			return model.Object{}, "", err
		}
		receipt, err = s.DB.InvitationUpload(ctx, i.ID, id, key)
		if err != nil {
			return model.Object{}, "", err
		}
	}
	// Re-read visibility: changing to private must never expose a new public URL.
	b, err := s.DB.GetBucket(ctx, c.Bucket.Slug)
	if err != nil {
		return o, "", err
	}
	publicURL := ""
	if b.Visibility == "public" && receipt.PublicSlug != "" {
		publicURL = s.Config.PublicURL + "/p/" + receipt.PublicSlug
	}
	return o, publicURL, nil
}
