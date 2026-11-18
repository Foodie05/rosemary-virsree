package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"rosemary-virsree/internal/store"
)

func invitationStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrInvitationInactive), errors.Is(err, store.ErrInvitationOwnership):
		return http.StatusForbidden
	case errors.Is(err, store.ErrInvitationLimit), errors.Is(err, store.ErrInvitationOverwrite):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}
func (s *Server) uploadInvitations(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	b, err := s.svc.DB.GetBucket(r.Context(), r.PathValue("bucket"))
	if err != nil {
		fail(w, r, 404, "virtual bucket not found")
		return
	}
	if r.Method == http.MethodGet {
		list, err := s.svc.DB.ListUploadInvitations(r.Context(), b.ID)
		if err != nil {
			fail(w, r, 500, err.Error())
			return
		}
		write(w, 200, list)
		return
	}
	var in struct {
		Name      string `json:"name"`
		MaxFiles  int64  `json:"max_files"`
		Expires   int64  `json:"expires_in"`
		Overwrite bool   `json:"allow_overwrite"`
	}
	if err = decode(r, &in); err != nil {
		fail(w, r, 400, err.Error())
		return
	}
	i, token, err := s.svc.CreateUploadInvitation(r.Context(), b, in.Name, in.MaxFiles, in.Expires, in.Overwrite)
	if err != nil {
		fail(w, r, invitationStatus(err), err.Error())
		return
	}
	write(w, 201, map[string]any{"token": token, "invitation": i, "upload_url": s.svc.Config.PublicURL + "/api/v1/agent/uploads", "bucket": b.Slug, "visibility": b.Visibility, "shown_once": true})
}
func (s *Server) revokeUploadInvitation(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	b, err := s.svc.DB.GetBucket(r.Context(), r.PathValue("bucket"))
	if err != nil {
		fail(w, r, 404, "virtual bucket not found")
		return
	}
	if err = s.svc.DB.RevokeUploadInvitation(r.Context(), b.ID, r.PathValue("id")); err != nil {
		fail(w, r, 404, "upload invitation not found")
		return
	}
	s.svc.DB.Audit(r.Context(), "upload-invitation.revoked", r.PathValue("id"), b.Slug)
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) invitedUpload(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		fail(w, r, 403, store.ErrInvitationInactive.Error())
		return
	}
	i, c, err := s.svc.AuthenticateUploadInvitation(r.Context(), raw)
	if err != nil {
		fail(w, r, invitationStatus(err), err.Error())
		return
	}
	if r.Method == http.MethodGet {
		write(w, 200, map[string]any{"bucket": c.Bucket.Slug, "visibility": c.Bucket.Visibility, "invitation": i})
		return
	}
	if strings.HasSuffix(r.URL.Path, "/commit") {
		var in struct {
			ID  string `json:"upload_id"`
			Key string `json:"key"`
		}
		if err = decode(r, &in); err != nil {
			fail(w, r, 400, err.Error())
			return
		}
		o, u, err := s.svc.CommitInvitedUpload(r.Context(), i, c, in.ID, in.Key)
		if err != nil {
			fail(w, r, invitationStatus(err), err.Error())
			return
		}
		write(w, 200, map[string]any{"object": o, "public_url": u})
		return
	}
	var in struct {
		Key         string `json:"key"`
		Size        *int64 `json:"size"`
		ContentType string `json:"content_type"`
	}
	if err = decode(r, &in); err != nil {
		fail(w, r, 400, err.Error())
		return
	}
	if in.Size == nil {
		fail(w, r, 400, "size is required")
		return
	}
	o, u, expires, err := s.svc.BeginInvitedUpload(r.Context(), i, c, in.Key, in.ContentType, *in.Size)
	if err != nil {
		fail(w, r, invitationStatus(err), err.Error())
		return
	}
	headers := map[string]string{}
	if in.ContentType != "" {
		headers["Content-Type"] = in.ContentType
	}
	write(w, 201, map[string]any{"expected_size": *in.Size, "upload_id": o.ID, "url": u, "method": "PUT", "required_headers": headers, "expires_in": expires, "direct": s.svc.Storage.Direct(o.SourceID), "commit_url": s.svc.Config.PublicURL + "/api/v1/agent/uploads/commit"})
}

// Keep invitation authorization separate from virtual credentials: it grants no
// read/list/delete capability and can never be used to obtain an application key.
