package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"rosemary-virsree/internal/model"
	"rosemary-virsree/internal/secretbox"
)

var (
	ErrInvitationInactive  = errors.New("upload_invitation_inactive")
	ErrInvitationOwnership = errors.New("upload_invitation_ownership")
	ErrInvitationLimit     = errors.New("upload_invitation_limit")
	ErrInvitationOverwrite = errors.New("upload_overwrite_denied")
)

type UploadInvitation struct {
	ID             string    `json:"id"`
	BucketID       string    `json:"-"`
	Name           string    `json:"name"`
	TokenHash      string    `json:"-"`
	MaxFiles       int64     `json:"max_files"`
	AllowOverwrite bool      `json:"allow_overwrite"`
	Revoked        bool      `json:"revoked"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
	UsedFiles      int64     `json:"used_files"`
	PendingFiles   int64     `json:"pending_files"`
}

type InvitationUpload struct {
	UploadID     string
	InvitationID string
	Key          string
	Status       string
	ObjectID     string
	PublicSlug   string
}

const invitationColumns = `i.id,i.bucket_id,i.name,i.token_hash,i.max_files,i.allow_overwrite,i.revoked,i.expires_at,i.created_at,
 (SELECT count(*) FROM invitation_uploads x WHERE x.invitation_id=i.id AND x.status='committed'),
 (SELECT count(*) FROM invitation_uploads x JOIN uploads u ON u.id=x.upload_id WHERE x.invitation_id=i.id AND x.status='pending' AND u.expires_at>?)`

func invitation(row interface{ Scan(...any) error }) (UploadInvitation, error) {
	var i UploadInvitation
	err := row.Scan(&i.ID, &i.BucketID, &i.Name, &i.TokenHash, &i.MaxFiles, &i.AllowOverwrite, &i.Revoked, &i.ExpiresAt, &i.CreatedAt, &i.UsedFiles, &i.PendingFiles)
	return i, err
}
func (s *Store) CreateUploadInvitation(ctx context.Context, i UploadInvitation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = requireActiveBucket(ctx, tx, i.BucketID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO upload_invitations(id,bucket_id,name,token_hash,max_files,allow_overwrite,revoked,expires_at,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, i.ID, i.BucketID, i.Name, i.TokenHash, i.MaxFiles, i.AllowOverwrite, i.Revoked, i.ExpiresAt, i.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ListUploadInvitations(ctx context.Context, bucketID string) ([]UploadInvitation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+invitationColumns+` FROM upload_invitations i WHERE i.bucket_id=? ORDER BY i.created_at DESC`, time.Now().UTC(), bucketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UploadInvitation{}
	for rows.Next() {
		i, e := invitation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *Store) UploadInvitation(ctx context.Context, id string) (UploadInvitation, error) {
	return invitation(s.db.QueryRowContext(ctx, `SELECT `+invitationColumns+` FROM upload_invitations i WHERE i.id=?`, time.Now().UTC(), id))
}
func (s *Store) UploadInvitationByHash(ctx context.Context, hash string) (UploadInvitation, error) {
	return invitation(s.db.QueryRowContext(ctx, `SELECT `+invitationColumns+` FROM upload_invitations i WHERE i.token_hash=?`, time.Now().UTC(), hash))
}
func (s *Store) InvitationBucketSlug(ctx context.Context, id string) (string, error) {
	var slug string
	err := s.db.QueryRowContext(ctx, `SELECT slug FROM buckets WHERE id=?`, id).Scan(&slug)
	return slug, err
}
func (s *Store) RevokeUploadInvitation(ctx context.Context, bucketID, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE upload_invitations SET revoked=1 WHERE bucket_id=? AND id=?`, bucketID, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}
func activeInvitation(ctx context.Context, tx *sql.Tx, id, bucketID string) (UploadInvitation, error) {
	i, err := invitation(tx.QueryRowContext(ctx, `SELECT `+invitationColumns+` FROM upload_invitations i WHERE i.id=?`, time.Now().UTC(), id))
	if err != nil || i.Revoked || !time.Now().Before(i.ExpiresAt) {
		return i, ErrInvitationInactive
	}
	if i.BucketID != bucketID {
		return i, ErrInvitationOwnership
	}
	return i, nil
}
func invitationOverwrite(ctx context.Context, tx *sql.Tx, i UploadInvitation, key string) error {
	if i.AllowOverwrite {
		return nil
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM objects WHERE bucket_id=? AND logical_key=? AND status='ready')`, i.BucketID, key).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrInvitationOverwrite
	}
	return nil
}
func invitationReserve(ctx context.Context, tx *sql.Tx, id string, o model.Object) error {
	i, err := activeInvitation(ctx, tx, id, o.BucketID)
	if err != nil {
		return err
	}
	if i.MaxFiles > 0 && i.UsedFiles+i.PendingFiles >= i.MaxFiles {
		return ErrInvitationLimit
	}
	return invitationOverwrite(ctx, tx, i, o.LogicalKey)
}
func invitationCommit(ctx context.Context, tx *sql.Tx, o model.Object) error {
	var id, key string
	err := tx.QueryRowContext(ctx, `SELECT invitation_id,object_key FROM invitation_uploads WHERE upload_id=? AND status='pending'`, o.ID).Scan(&id, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if key != o.LogicalKey {
		return ErrInvitationOwnership
	}
	i, err := activeInvitation(ctx, tx, id, o.BucketID)
	if err != nil {
		return err
	}
	// This pending reservation already owns a slot; commits must not count it twice.
	if i.MaxFiles > 0 && i.UsedFiles >= i.MaxFiles {
		return ErrInvitationLimit
	}
	return invitationOverwrite(ctx, tx, i, o.LogicalKey)
}
func invitationReceipt(ctx context.Context, tx *sql.Tx, uploadID, objectID, bucketID string) error {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT invitation_id FROM invitation_uploads WHERE upload_id=?`, uploadID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var visibility string
	if err = tx.QueryRowContext(ctx, `SELECT visibility FROM buckets WHERE id=?`, bucketID).Scan(&visibility); err != nil {
		return err
	}
	slug := ""
	if visibility == "public" {
		slug = secretbox.Random("pub_", 18)
		// Stable aliases refresh a short storage URL on each visit; they do not expire.
		_, err = tx.ExecContext(ctx, `INSERT INTO public_links(id,object_id,slug,sign_ttl_seconds,created_at) VALUES(?,?,?,?,?)`, secretbox.Random("lnk_", 10), objectID, slug, 600, time.Now().UTC())
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE invitation_uploads SET status='committed',object_id=?,public_slug=? WHERE upload_id=?`, objectID, slug, uploadID)
	return err
}
func (s *Store) InvitationUpload(ctx context.Context, id, uploadID, key string) (InvitationUpload, error) {
	var x InvitationUpload
	err := s.db.QueryRowContext(ctx, `SELECT upload_id,invitation_id,object_key,status,object_id,public_slug FROM invitation_uploads WHERE invitation_id=? AND upload_id=? AND object_key=?`, id, uploadID, key).Scan(&x.UploadID, &x.InvitationID, &x.Key, &x.Status, &x.ObjectID, &x.PublicSlug)
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrInvitationOwnership
	}
	return x, err
}
func (s *Store) RequireLiveInvitationUpload(ctx context.Context, id, uploadID, key string) (InvitationUpload, error) {
	i, err := s.UploadInvitation(ctx, id)
	if err != nil || i.Revoked || !time.Now().Before(i.ExpiresAt) {
		return InvitationUpload{}, ErrInvitationInactive
	}
	if err = s.RequireActiveBucket(ctx, i.BucketID); err != nil {
		return InvitationUpload{}, err
	}
	return s.InvitationUpload(ctx, id, uploadID, key)
}
