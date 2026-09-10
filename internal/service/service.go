package service

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"rosemary-virsree/internal/config"
	"rosemary-virsree/internal/model"
	"rosemary-virsree/internal/provider"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/store"
)

var slugRx = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

type Service struct {
	DB         *store.Store
	Storage    *StorageManager
	Box        *secretbox.Box
	Config     config.Config
	PlatformFS *PlatformFilesystem
}
type Credential struct {
	AccessKey, SecretKey string `json:"-"`
	Bucket               model.Bucket
	Key                  model.AccessKey
}

func New(db *store.Store, p *provider.S3, b *secretbox.Box, c config.Config) (*Service, error) {
	m := NewStorageManager(db, b, c.PublicURL, p)
	if err := m.Load(context.Background()); err != nil {
		return nil, fmt.Errorf("load storage sources: %w", err)
	}
	s := &Service{DB: db, Storage: m, Box: b, Config: c}
	s.PlatformFS = NewPlatformFilesystem(db, m, b, time.Duration(c.BackupInterval)*time.Second, c.BackupRetention)
	return s, nil
}
func (s *Service) NewBucket(ctx context.Context, name, slug, visibility string, quota int64, unlimited bool) (model.Bucket, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugRx.MatchString(slug) {
		return model.Bucket{}, errors.New("slug must be 3-63 lowercase letters, numbers or hyphens")
	}
	if visibility != "private" && visibility != "public" {
		return model.Bucket{}, errors.New("visibility must be private or public")
	}
	b := model.Bucket{ID: secretbox.Random("bkt_", 12), Name: strings.TrimSpace(name), Slug: slug, Visibility: visibility, QuotaBytes: quota, QuotaUnlimited: unlimited, CreatedAt: time.Now().UTC()}
	b, e := s.DB.CreateBucket(ctx, b, s.Config.TotalQuota, s.Storage.PrimaryUnlimited())
	if e == nil {
		s.DB.Audit(ctx, "bucket.created", slug, fmt.Sprintf("quota=%d unlimited=%t visibility=%s", quota, unlimited, visibility))
	}
	return b, e
}

func (s *Service) UpdateBucket(ctx context.Context, slug, name, visibility string, quota int64, unlimited bool) (model.Bucket, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Bucket{}, errors.New("name is required")
	}
	if visibility != "private" && visibility != "public" {
		return model.Bucket{}, errors.New("visibility must be private or public")
	}
	b, err := s.DB.UpdateBucket(ctx, slug, name, visibility, quota, unlimited, s.Config.TotalQuota, s.Storage.PrimaryUnlimited())
	if err == nil {
		s.DB.Audit(ctx, "bucket.updated", slug, fmt.Sprintf("quota=%d unlimited=%t visibility=%s", quota, unlimited, visibility))
	}
	return b, err
}

func (s *Service) BucketDetail(ctx context.Context, slug string) (map[string]any, error) {
	b, err := s.DB.GetBucket(ctx, slug)
	if err != nil {
		return nil, err
	}
	metrics, err := s.DB.BucketMetrics(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	allocations, err := s.DB.BucketAllocations(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	region := s.Config.Backend.Region
	if region == "" {
		region = "us-east-1"
	}
	return map[string]any{
		"bucket": b, "status": map[bool]string{true: "ready", false: "storage_unavailable"}[s.Storage.Ready()],
		"s3_endpoint": s.Config.PublicURL + "/s3", "api_endpoint": s.Config.PublicURL + "/api/v1/buckets/" + b.Slug,
		"region": region, "can_set_unlimited": s.Storage.PrimaryUnlimited(), "metrics": metrics, "allocations": allocations,
	}, nil
}
func (s *Service) NewKey(ctx context.Context, b model.Bucket, name, perms string) (model.AccessKey, string, error) {
	if perms == "" {
		perms = "read,write,delete,manage"
	}
	seen := map[string]bool{}
	var normalized []string
	for _, permission := range strings.Split(perms, ",") {
		permission = strings.TrimSpace(permission)
		if permission != "read" && permission != "write" && permission != "delete" && permission != "manage" {
			return model.AccessKey{}, "", fmt.Errorf("unknown permission %q", permission)
		}
		if !seen[permission] {
			seen[permission] = true
			normalized = append(normalized, permission)
		}
	}
	if len(normalized) == 0 {
		return model.AccessKey{}, "", errors.New("at least one permission is required")
	}
	perms = strings.Join(normalized, ",")
	secret := secretbox.Random("rvs_", 30)
	cipher, e := s.Box.Seal(secret)
	if e != nil {
		return model.AccessKey{}, "", e
	}
	k := model.AccessKey{ID: secretbox.Random("key_", 10), BucketID: b.ID, Name: name, AK: secretbox.Random("RVS", 12), SecretCipher: cipher, Permissions: perms, CreatedAt: time.Now().UTC()}
	e = s.DB.CreateAccessKey(ctx, k)
	if e == nil {
		s.DB.Audit(ctx, "access-key.created", b.Slug, k.AK)
	}
	return k, secret, e
}
func (s *Service) Authenticate(ctx context.Context, ak, secret, permission string) (Credential, error) {
	k, b, e := s.DB.AccessByAK(ctx, ak)
	if e != nil {
		return Credential{}, errors.New("invalid credentials")
	}
	if k.Revoked {
		return Credential{}, errors.New("credential revoked")
	}
	plain, e := s.Box.Open(k.SecretCipher)
	if e != nil || subtle.ConstantTimeCompare([]byte(plain), []byte(secret)) != 1 {
		return Credential{}, errors.New("invalid credentials")
	}
	if !has(k.Permissions, permission) {
		return Credential{}, errors.New("permission denied")
	}
	return Credential{AccessKey: ak, SecretKey: plain, Bucket: b, Key: k}, nil
}
func has(csv, v string) bool {
	for _, p := range strings.Split(csv, ",") {
		if strings.TrimSpace(p) == v {
			return true
		}
	}
	return false
}
func (s *Service) CreateBootstrap(ctx context.Context, name string, maxQuota int64, ttl time.Duration) (string, model.BootstrapToken, error) {
	if ttl < time.Minute {
		return "", model.BootstrapToken{}, errors.New("bootstrap token lifetime must be at least one minute")
	}
	if maxQuota <= 0 {
		return "", model.BootstrapToken{}, errors.New("bootstrap maximum quota must be greater than zero")
	}
	raw := secretbox.Random("rvs_boot_", 24)
	t := model.BootstrapToken{ID: secretbox.Random("boot_", 10), Name: name, TokenHash: secretbox.Hash(raw), MaxQuota: maxQuota, ExpiresAt: time.Now().Add(ttl).UTC()}
	e := s.DB.CreateBootstrap(ctx, t)
	if e == nil {
		s.DB.Audit(ctx, "bootstrap.created", t.ID, name)
	}
	return raw, t, e
}
func (s *Service) Claim(ctx context.Context, token, name, slug, visibility string, quota int64) (model.Bucket, model.AccessKey, string, error) {
	hash := secretbox.Hash(token)
	t, e := s.DB.InspectBootstrap(ctx, hash)
	if e != nil {
		return model.Bucket{}, model.AccessKey{}, "", e
	}
	if quota > t.MaxQuota {
		return model.Bucket{}, model.AccessKey{}, "", errors.New("requested quota exceeds bootstrap allowance")
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	if quota <= 0 || !slugRx.MatchString(slug) || (visibility != "private" && visibility != "public") {
		return model.Bucket{}, model.AccessKey{}, "", errors.New("invalid virtual bucket request")
	}
	if _, bucketErr := s.DB.GetBucket(ctx, slug); bucketErr == nil {
		return model.Bucket{}, model.AccessKey{}, "", errors.New("virtual bucket already exists")
	} else if !errors.Is(bucketErr, sql.ErrNoRows) {
		return model.Bucket{}, model.AccessKey{}, "", bucketErr
	}
	if _, e = s.DB.ClaimBootstrap(ctx, hash); e != nil {
		return model.Bucket{}, model.AccessKey{}, "", e
	}
	b, e := s.NewBucket(ctx, name, slug, visibility, quota, false)
	if e != nil {
		return b, model.AccessKey{}, "", e
	}
	k, secret, e := s.NewKey(ctx, b, "owner", "read,write,delete,manage")
	return b, k, secret, e
}
func physical(b model.Bucket, key string, generation int64) string {
	return fmt.Sprintf("rosemary/%s/g%d/%s", b.ID, generation, strings.TrimPrefix(key, "/"))
}
func stagingPhysical(b model.Bucket, uploadID, key string) string {
	return fmt.Sprintf("rosemary-staging/%s/%s/%s", b.ID, uploadID, strings.TrimPrefix(key, "/"))
}
func (s *Service) BeginUpload(ctx context.Context, c Credential, key, contentType string, size, expires int64) (model.Object, string, error) {
	if size < 0 {
		return model.Object{}, "", errors.New("size is required")
	}
	key = strings.TrimPrefix(key, "/")
	if key == "" || len([]byte(key)) > 1024 {
		return model.Object{}, "", errors.New("key must contain 1-1024 UTF-8 bytes")
	}
	if expires < 1 {
		return model.Object{}, "", errors.New("expires_in is required")
	}
	s.cleanupExpiredUploads(ctx)
	gen := time.Now().UnixNano()
	o := model.Object{ID: secretbox.Random("obj_", 12), BucketID: c.Bucket.ID, LogicalKey: key, Size: size, ContentType: contentType, Generation: gen, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	var b provider.Backend
	var e error
	for _, candidate := range s.Storage.candidates() {
		o.SourceID = candidate.meta.ID
		o.PhysicalKey = stagingPhysical(c.Bucket, o.ID, o.LogicalKey)
		if e = s.DB.ReserveObject(ctx, o, time.Now().Add(time.Duration(expires)*time.Second), s.Config.MaxObjectsPerBucket, s.Config.MaxPendingUploads); e == nil {
			b = candidate.backend
			break
		}
		if !strings.Contains(e.Error(), "storage source capacity") {
			return o, "", e
		}
	}
	if b == nil {
		return o, "", errors.New("no storage source has enough capacity")
	}
	var u string
	if b.Kind() == "s3" {
		u, e = b.PresignPut(ctx, o.PhysicalKey, contentType, size, time.Duration(expires)*time.Second)
	} else {
		token := secretbox.Random("rvs_up_", 24)
		e = s.DB.SetUploadTransferHash(ctx, o.ID, secretbox.Hash(token))
		u = s.Config.PublicURL + "/transfer/upload/" + token
	}
	if e != nil {
		_ = s.DB.CancelUpload(ctx, o.ID)
		return o, "", e
	}
	s.DB.Audit(ctx, "upload.signed", c.Bucket.Slug, o.LogicalKey)
	return o, u, nil
}

func (s *Service) cleanupExpiredUploads(ctx context.Context) {
	uploads, err := s.DB.ExpiredUploads(ctx, 100)
	if err != nil {
		return
	}
	for _, upload := range uploads {
		backend, err := s.Storage.backend(upload.SourceID)
		if err != nil {
			continue
		}
		if err = backend.Delete(ctx, upload.PhysicalKey); err != nil {
			continue
		}
		_ = s.DB.CancelUpload(ctx, upload.ID)
	}
}
func (s *Service) CommitUpload(ctx context.Context, c Credential, id, key string) (model.Object, error) {
	o, e := s.DB.GetUpload(ctx, c.Bucket.ID, id, key)
	if e != nil {
		return o, e
	}
	b, e := s.Storage.backend(o.SourceID)
	if e != nil {
		return o, e
	}
	h, e := b.Head(ctx, o.PhysicalKey)
	if e != nil {
		return o, e
	}
	if h.Size != o.Size {
		return o, fmt.Errorf("uploaded size %d does not match reserved size %d", h.Size, o.Size)
	}
	stagingPhysical := o.PhysicalKey
	finalGeneration := time.Now().UnixNano()
	if finalGeneration == o.Generation {
		finalGeneration++
	}
	finalPhysical := physical(c.Bucket, o.LogicalKey, finalGeneration)
	o.PhysicalKey = finalPhysical
	if e = b.Copy(ctx, stagingPhysical, finalPhysical); e != nil {
		return o, fmt.Errorf("promote staged upload: %w", e)
	}
	finalHead, e := b.Head(ctx, finalPhysical)
	if e != nil || finalHead.Size != h.Size {
		_ = b.Delete(ctx, finalPhysical)
		if e != nil {
			return o, fmt.Errorf("verify promoted upload: %w", e)
		}
		return o, errors.New("promoted upload size mismatch")
	}
	o, oldPhysical, oldSourceID, e := s.DB.CommitUpload(ctx, o, finalHead.ETag, finalHead.Size)
	if e != nil {
		_ = b.Delete(ctx, finalPhysical)
		return o, e
	}
	_ = b.Delete(ctx, stagingPhysical)
	if oldPhysical != "" && oldPhysical != o.PhysicalKey {
		oldBackend, _ := s.Storage.backend(oldSourceID)
		if oldBackend != nil {
			_ = oldBackend.Delete(ctx, oldPhysical)
		}
	}
	s.DB.Audit(ctx, "upload.committed", c.Bucket.Slug, o.LogicalKey)
	return o, e
}
func (s *Service) DownloadURL(ctx context.Context, c Credential, key string, expires int64, filename string) (string, error) {
	if expires < 1 {
		return "", errors.New("expires_in is required")
	}
	o, e := s.DB.GetObject(ctx, c.Bucket.ID, key)
	if e != nil {
		return "", e
	}
	if o.Status != "ready" {
		return "", errors.New("object is not ready")
	}
	b, e := s.Storage.backend(o.SourceID)
	if e != nil {
		return "", e
	}
	if b.Kind() == "s3" {
		return b.PresignGet(ctx, o.PhysicalKey, time.Duration(expires)*time.Second, filename)
	}
	token := secretbox.Random("rvs_dl_", 24)
	if e = s.DB.CreateDownloadToken(ctx, secretbox.Hash(token), o.ID, time.Now().Add(time.Duration(expires)*time.Second)); e != nil {
		return "", e
	}
	return s.Config.PublicURL + "/transfer/download/" + token, nil
}
func (s *Service) Delete(ctx context.Context, c Credential, key string) error {
	o, e := s.DB.GetObject(ctx, c.Bucket.ID, key)
	if e != nil {
		return e
	}
	b, e := s.Storage.backend(o.SourceID)
	if e != nil {
		return e
	}
	if e = b.Delete(ctx, o.PhysicalKey); e != nil {
		return e
	}
	e = s.DB.DeleteObject(ctx, o.ID)
	if e == nil {
		s.DB.Audit(ctx, "object.deleted", c.Bucket.Slug, key)
	}
	return e
}
func (s *Service) NewPublicLink(ctx context.Context, c Credential, key string, signTTL, linkTTL int64) (string, string, string, error) {
	if signTTL < 1 {
		return "", "", "", errors.New("sign_expires_in is required")
	}
	if linkTTL < 0 {
		return "", "", "", errors.New("link_expires_in cannot be negative")
	}
	o, e := s.DB.GetObject(ctx, c.Bucket.ID, key)
	if e != nil {
		return "", "", "", e
	}
	if o.Status != "ready" {
		return "", "", "", errors.New("object is not ready")
	}
	b, e := s.Storage.backend(o.SourceID)
	if e != nil {
		return "", "", "", e
	}
	var direct string
	if b.Kind() == "s3" {
		direct, e = b.PresignGet(ctx, o.PhysicalKey, time.Duration(signTTL)*time.Second, "")
	} else {
		token := secretbox.Random("rvs_dl_", 24)
		e = s.DB.CreateDownloadToken(ctx, secretbox.Hash(token), o.ID, time.Now().Add(time.Duration(signTTL)*time.Second))
		direct = s.Config.PublicURL + "/transfer/download/" + token
	}
	if e != nil {
		return "", "", "", e
	}
	slug := secretbox.Random("pub_", 18)
	var expiry *time.Time
	if linkTTL > 0 {
		t := time.Now().Add(time.Duration(linkTTL) * time.Second).UTC()
		expiry = &t
	}
	if e = s.DB.CreatePublicLink(ctx, secretbox.Random("lnk_", 10), o.ID, slug, signTTL, expiry); e != nil {
		return "", "", "", e
	}
	s.DB.Audit(ctx, "public-link.created", c.Bucket.Slug, key)
	return slug, s.Config.PublicURL + "/p/" + slug, direct, nil
}
func (s *Service) RevokePublicLink(ctx context.Context, c Credential, slug string) error {
	if e := s.DB.RevokePublicLink(ctx, c.Bucket.ID, slug); e != nil {
		return e
	}
	s.DB.Audit(ctx, "public-link.revoked", c.Bucket.Slug, slug)
	return nil
}
func (s *Service) ResolvePublic(ctx context.Context, slug string) (string, error) {
	o, ttl, e := s.DB.PublicObject(ctx, slug)
	if e != nil {
		return "", e
	}
	b, e := s.Storage.backend(o.SourceID)
	if e != nil {
		return "", e
	}
	if b.Kind() == "s3" {
		return b.PresignGet(ctx, o.PhysicalKey, time.Duration(ttl)*time.Second, "")
	}
	token := secretbox.Random("rvs_dl_", 24)
	if e = s.DB.CreateDownloadToken(ctx, secretbox.Hash(token), o.ID, time.Now().Add(time.Duration(ttl)*time.Second)); e != nil {
		return "", e
	}
	return s.Config.PublicURL + "/transfer/download/" + token, nil
}
func (s *Service) RelayUpload(ctx context.Context, token string, body io.Reader, size int64) error {
	return s.Storage.relayUpload(ctx, token, body, size)
}
func (s *Service) RelayDownload(ctx context.Context, token string) (io.ReadCloser, provider.Head, error) {
	return s.Storage.relayDownload(ctx, token)
}
func IsNotFound(e error) bool { return errors.Is(e, sql.ErrNoRows) }
