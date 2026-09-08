package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"rosemary-virsree/internal/model"
)

func (s *Store) CreateStorageSource(ctx context.Context, v model.StorageSource) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO storage_sources(id,name,kind,priority,capacity_bytes,enabled,direct_transfer,cdn_enabled,config_cipher,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, v.ID, v.Name, v.Kind, v.Priority, v.CapacityBytes, v.Enabled, v.Direct, v.CDNEnabled, v.ConfigCipher, v.CreatedAt)
	return e
}
func (s *Store) ListStorageSources(ctx context.Context) ([]model.StorageSource, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,name,kind,priority,capacity_bytes,used_bytes,reserved_bytes,enabled,direct_transfer,cdn_enabled,config_cipher,created_at FROM storage_sources ORDER BY priority,created_at,id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.StorageSource
	for rows.Next() {
		var v model.StorageSource
		if e = rows.Scan(&v.ID, &v.Name, &v.Kind, &v.Priority, &v.CapacityBytes, &v.UsedBytes, &v.ReservedBytes, &v.Enabled, &v.Direct, &v.CDNEnabled, &v.ConfigCipher, &v.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) GetStorageSource(ctx context.Context, id string) (model.StorageSource, error) {
	var v model.StorageSource
	e := s.db.QueryRowContext(ctx, `SELECT id,name,kind,priority,capacity_bytes,used_bytes,reserved_bytes,enabled,direct_transfer,cdn_enabled,config_cipher,created_at FROM storage_sources WHERE id=?`, id).Scan(&v.ID, &v.Name, &v.Kind, &v.Priority, &v.CapacityBytes, &v.UsedBytes, &v.ReservedBytes, &v.Enabled, &v.Direct, &v.CDNEnabled, &v.ConfigCipher, &v.CreatedAt)
	return v, e
}
func (s *Store) StorageConfigured(ctx context.Context) bool {
	var n int
	_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM storage_sources WHERE enabled=1").Scan(&n)
	return n > 0
}

func (s *Store) SaveOIDCChallenge(ctx context.Context, stateHash, nonce, verifier string, expires time.Time) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO oidc_challenges(state_hash,nonce,verifier,expires_at,created_at) VALUES(?,?,?,?,?)", stateHash, nonce, verifier, expires, time.Now().UTC())
	return e
}
func (s *Store) ConsumeOIDCChallenge(ctx context.Context, stateHash string) (nonce, verifier string, e error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return "", "", e
	}
	defer tx.Rollback()
	var exp time.Time
	e = tx.QueryRowContext(ctx, "SELECT nonce,verifier,expires_at FROM oidc_challenges WHERE state_hash=?", stateHash).Scan(&nonce, &verifier, &exp)
	if e != nil {
		return "", "", e
	}
	if time.Now().After(exp) {
		return "", "", errors.New("OIDC challenge expired")
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM oidc_challenges WHERE state_hash=?", stateHash); e != nil {
		return "", "", e
	}
	e = tx.Commit()
	return
}
func (s *Store) CreateAdminSession(ctx context.Context, hash, email string, expires time.Time) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO admin_sessions(token_hash,email,expires_at,created_at) VALUES(?,?,?,?)", hash, email, expires, time.Now().UTC())
	return e
}
func (s *Store) AdminSession(ctx context.Context, hash string) (string, error) {
	var email string
	var exp time.Time
	e := s.db.QueryRowContext(ctx, "SELECT email,expires_at FROM admin_sessions WHERE token_hash=?", hash).Scan(&email, &exp)
	if e != nil {
		return "", e
	}
	if time.Now().After(exp) {
		_, _ = s.db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE token_hash=?", hash)
		return "", sql.ErrNoRows
	}
	return email, nil
}
func (s *Store) DeleteAdminSession(ctx context.Context, hash string) {
	_, _ = s.db.ExecContext(ctx, "DELETE FROM admin_sessions WHERE token_hash=?", hash)
}

func (s *Store) SetUploadTransferHash(ctx context.Context, id, hash string) error {
	_, e := s.db.ExecContext(ctx, "UPDATE uploads SET transfer_hash=? WHERE id=?", hash, id)
	return e
}
func (s *Store) UploadByTransferHash(ctx context.Context, hash string) (model.Object, time.Time, error) {
	var o model.Object
	var exp time.Time
	e := s.db.QueryRowContext(ctx, `SELECT id,bucket_id,source_id,logical_key,physical_key,size,content_type,created_at,expires_at FROM uploads WHERE transfer_hash=?`, hash).Scan(&o.ID, &o.BucketID, &o.SourceID, &o.LogicalKey, &o.PhysicalKey, &o.Size, &o.ContentType, &o.CreatedAt, &exp)
	return o, exp, e
}
func (s *Store) CreateDownloadToken(ctx context.Context, hash, objectID string, expires time.Time) error {
	_, e := s.db.ExecContext(ctx, `INSERT INTO transfer_tokens(token_hash,object_id,source_id,physical_key,size,content_type,etag,mode,expires_at,created_at)
		SELECT ?,id,source_id,physical_key,size,COALESCE(content_type,''),COALESCE(etag,''),'download',?,? FROM objects WHERE id=? AND status='ready'`, hash, expires, time.Now().UTC(), objectID)
	return e
}
func (s *Store) DownloadByTransferHash(ctx context.Context, hash string) (model.Object, time.Time, error) {
	var o model.Object
	var exp time.Time
	e := s.db.QueryRowContext(ctx, `SELECT o.id,o.bucket_id,COALESCE(NULLIF(t.source_id,''),o.source_id),o.logical_key,COALESCE(NULLIF(t.physical_key,''),o.physical_key),CASE WHEN t.physical_key='' THEN o.size ELSE t.size END,CASE WHEN t.physical_key='' THEN o.content_type ELSE t.content_type END,CASE WHEN t.physical_key='' THEN o.etag ELSE t.etag END,o.status,o.generation,o.is_public,o.created_at,o.updated_at,t.expires_at FROM transfer_tokens t JOIN objects o ON o.id=t.object_id WHERE t.token_hash=? AND t.mode='download'`, hash).Scan(&o.ID, &o.BucketID, &o.SourceID, &o.LogicalKey, &o.PhysicalKey, &o.Size, &o.ContentType, &o.ETag, &o.Status, &o.Generation, &o.Public, &o.CreatedAt, &o.UpdatedAt, &exp)
	return o, exp, e
}
