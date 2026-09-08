package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"rosemary-virsree/internal/model"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err = s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err = os.Chmod(path, 0600); err != nil {
			db.Close()
			return nil, fmt.Errorf("protect database file: %w", err)
		}
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS buckets(id TEXT PRIMARY KEY,name TEXT NOT NULL,slug TEXT NOT NULL UNIQUE,visibility TEXT NOT NULL DEFAULT 'private',quota_bytes INTEGER NOT NULL,used_bytes INTEGER NOT NULL DEFAULT 0,reserved_bytes INTEGER NOT NULL DEFAULT 0,created_at DATETIME NOT NULL);
CREATE TABLE IF NOT EXISTS access_keys(id TEXT PRIMARY KEY,bucket_id TEXT NOT NULL,name TEXT NOT NULL,ak TEXT NOT NULL UNIQUE,secret_cipher TEXT NOT NULL,permissions TEXT NOT NULL,revoked INTEGER NOT NULL DEFAULT 0,created_at DATETIME NOT NULL,last_used_at DATETIME,FOREIGN KEY(bucket_id) REFERENCES buckets(id));
CREATE TABLE IF NOT EXISTS objects(id TEXT PRIMARY KEY,bucket_id TEXT NOT NULL,logical_key TEXT NOT NULL,physical_key TEXT NOT NULL UNIQUE,size INTEGER NOT NULL,content_type TEXT,etag TEXT,status TEXT NOT NULL,generation INTEGER NOT NULL DEFAULT 1,is_public INTEGER NOT NULL DEFAULT 0,created_at DATETIME NOT NULL,updated_at DATETIME NOT NULL,UNIQUE(bucket_id,logical_key),FOREIGN KEY(bucket_id) REFERENCES buckets(id));
CREATE TABLE IF NOT EXISTS uploads(id TEXT PRIMARY KEY,bucket_id TEXT NOT NULL,logical_key TEXT NOT NULL,physical_key TEXT NOT NULL UNIQUE,size INTEGER NOT NULL,content_type TEXT,reserved_bytes INTEGER NOT NULL,expires_at DATETIME NOT NULL,created_at DATETIME NOT NULL,UNIQUE(bucket_id,logical_key),FOREIGN KEY(bucket_id) REFERENCES buckets(id));
CREATE TABLE IF NOT EXISTS bootstrap_tokens(id TEXT PRIMARY KEY,name TEXT NOT NULL,token_hash TEXT NOT NULL UNIQUE,max_quota INTEGER NOT NULL,expires_at DATETIME NOT NULL,used_at DATETIME,created_at DATETIME NOT NULL);
CREATE TABLE IF NOT EXISTS public_links(id TEXT PRIMARY KEY,object_id TEXT NOT NULL,slug TEXT NOT NULL UNIQUE,sign_ttl_seconds INTEGER NOT NULL,expires_at DATETIME,revoked INTEGER NOT NULL DEFAULT 0,created_at DATETIME NOT NULL,FOREIGN KEY(object_id) REFERENCES objects(id));
CREATE TABLE IF NOT EXISTS audits(id INTEGER PRIMARY KEY AUTOINCREMENT,action TEXT NOT NULL,subject TEXT NOT NULL,detail TEXT NOT NULL,created_at DATETIME NOT NULL);
CREATE INDEX IF NOT EXISTS objects_bucket ON objects(bucket_id,status);
CREATE INDEX IF NOT EXISTS uploads_expiry ON uploads(expires_at);
CREATE INDEX IF NOT EXISTS links_slug ON public_links(slug,revoked);
`)
	return err
}

func (s *Store) Audit(ctx context.Context, action, subject, detail string) {
	_, _ = s.db.ExecContext(ctx, "INSERT INTO audits(action,subject,detail,created_at) VALUES(?,?,?,?)", action, subject, detail, time.Now().UTC())
}
func (s *Store) Overview(ctx context.Context) (map[string]any, error) {
	var buckets, objects, keys int
	var used, reserved, quota int64
	err := s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM buckets),(SELECT count(*) FROM objects WHERE status='ready'),(SELECT count(*) FROM access_keys WHERE revoked=0),COALESCE((SELECT sum(used_bytes) FROM buckets),0),COALESCE((SELECT sum(reserved_bytes) FROM buckets),0),COALESCE((SELECT sum(quota_bytes) FROM buckets),0)`).Scan(&buckets, &objects, &keys, &used, &reserved, &quota)
	return map[string]any{"bucket_count": buckets, "object_count": objects, "active_key_count": keys, "used_bytes": used, "reserved_bytes": reserved, "allocated_quota": quota}, err
}
func (s *Store) ListBuckets(ctx context.Context) ([]model.Bucket, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT id,name,slug,visibility,quota_bytes,used_bytes,reserved_bytes,created_at FROM buckets ORDER BY created_at DESC")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Bucket
	for rows.Next() {
		var b model.Bucket
		if e = rows.Scan(&b.ID, &b.Name, &b.Slug, &b.Visibility, &b.QuotaBytes, &b.UsedBytes, &b.ReservedBytes, &b.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *Store) GetBucket(ctx context.Context, slug string) (model.Bucket, error) {
	var b model.Bucket
	e := s.db.QueryRowContext(ctx, "SELECT id,name,slug,visibility,quota_bytes,used_bytes,reserved_bytes,created_at FROM buckets WHERE slug=?", slug).Scan(&b.ID, &b.Name, &b.Slug, &b.Visibility, &b.QuotaBytes, &b.UsedBytes, &b.ReservedBytes, &b.CreatedAt)
	return b, e
}
func (s *Store) CreateBucket(ctx context.Context, b model.Bucket, totalLimit int64) (model.Bucket, error) {
	if b.QuotaBytes <= 0 {
		return b, errors.New("quota must be greater than zero")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return b, e
	}
	defer tx.Rollback()
	var allocated int64
	if e = tx.QueryRowContext(ctx, "SELECT COALESCE(sum(quota_bytes),0) FROM buckets").Scan(&allocated); e != nil {
		return b, e
	}
	if allocated+b.QuotaBytes > totalLimit {
		return b, fmt.Errorf("total quota exceeded")
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO buckets(id,name,slug,visibility,quota_bytes,created_at) VALUES(?,?,?,?,?,?)", b.ID, b.Name, b.Slug, b.Visibility, b.QuotaBytes, b.CreatedAt)
	if e != nil {
		return b, e
	}
	return b, tx.Commit()
}
func (s *Store) CreateAccessKey(ctx context.Context, k model.AccessKey) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO access_keys(id,bucket_id,name,ak,secret_cipher,permissions,created_at) VALUES(?,?,?,?,?,?,?)", k.ID, k.BucketID, k.Name, k.AK, k.SecretCipher, k.Permissions, k.CreatedAt)
	return e
}
func (s *Store) AccessByAK(ctx context.Context, ak string) (model.AccessKey, model.Bucket, error) {
	var k model.AccessKey
	var b model.Bucket
	var lastUsed sql.NullTime
	e := s.db.QueryRowContext(ctx, `SELECT k.id,k.bucket_id,k.name,k.ak,k.secret_cipher,k.permissions,k.revoked,k.created_at,k.last_used_at,b.id,b.name,b.slug,b.visibility,b.quota_bytes,b.used_bytes,b.reserved_bytes,b.created_at FROM access_keys k JOIN buckets b ON b.id=k.bucket_id WHERE k.ak=?`, ak).Scan(&k.ID, &k.BucketID, &k.Name, &k.AK, &k.SecretCipher, &k.Permissions, &k.Revoked, &k.CreatedAt, &lastUsed, &b.ID, &b.Name, &b.Slug, &b.Visibility, &b.QuotaBytes, &b.UsedBytes, &b.ReservedBytes, &b.CreatedAt)
	if lastUsed.Valid {
		k.LastUsedAt = lastUsed.Time
	}
	return k, b, e
}
func (s *Store) ListKeys(ctx context.Context) ([]map[string]any, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT k.id,k.name,k.ak,k.permissions,k.revoked,k.created_at,b.slug FROM access_keys k JOIN buckets b ON b.id=k.bucket_id ORDER BY k.created_at DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, n, ak, p, slug string
		var rev bool
		var t time.Time
		if e = rows.Scan(&id, &n, &ak, &p, &rev, &t, &slug); e != nil {
			return nil, e
		}
		out = append(out, map[string]any{"id": id, "name": n, "access_key": ak, "permissions": strings.Split(p, ","), "revoked": rev, "created_at": t, "bucket": slug})
	}
	return out, rows.Err()
}
func (s *Store) RevokeKey(ctx context.Context, id string) error {
	_, e := s.db.ExecContext(ctx, "UPDATE access_keys SET revoked=1 WHERE id=?", id)
	return e
}
func (s *Store) CreateBootstrap(ctx context.Context, t model.BootstrapToken) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO bootstrap_tokens(id,name,token_hash,max_quota,expires_at,created_at) VALUES(?,?,?,?,?,?)", t.ID, t.Name, t.TokenHash, t.MaxQuota, t.ExpiresAt, time.Now().UTC())
	return e
}
func (s *Store) InspectBootstrap(ctx context.Context, hash string) (model.BootstrapToken, error) {
	var t model.BootstrapToken
	var used sql.NullTime
	e := s.db.QueryRowContext(ctx, "SELECT id,name,token_hash,max_quota,expires_at,used_at FROM bootstrap_tokens WHERE token_hash=?", hash).Scan(&t.ID, &t.Name, &t.TokenHash, &t.MaxQuota, &t.ExpiresAt, &used)
	if e != nil {
		return t, e
	}
	if used.Valid || time.Now().After(t.ExpiresAt) {
		return t, errors.New("bootstrap token expired or already used")
	}
	return t, nil
}
func (s *Store) ClaimBootstrap(ctx context.Context, hash string) (model.BootstrapToken, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return model.BootstrapToken{}, e
	}
	defer tx.Rollback()
	var t model.BootstrapToken
	var used sql.NullTime
	e = tx.QueryRowContext(ctx, "SELECT id,name,token_hash,max_quota,expires_at,used_at FROM bootstrap_tokens WHERE token_hash=?", hash).Scan(&t.ID, &t.Name, &t.TokenHash, &t.MaxQuota, &t.ExpiresAt, &used)
	if e != nil {
		return t, e
	}
	if used.Valid || time.Now().After(t.ExpiresAt) {
		return t, errors.New("bootstrap token expired or already used")
	}
	now := time.Now().UTC()
	if _, e = tx.ExecContext(ctx, "UPDATE bootstrap_tokens SET used_at=? WHERE id=? AND used_at IS NULL", now, t.ID); e != nil {
		return t, e
	}
	t.UsedAt = &now
	return t, tx.Commit()
}
func (s *Store) ReserveObject(ctx context.Context, o model.Object, expiresAt time.Time, maxObjects, maxPending int64) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Release reservations abandoned after their signed upload URL expired.
	rows, e := tx.QueryContext(ctx, "SELECT bucket_id,COALESCE(sum(reserved_bytes),0) FROM uploads WHERE expires_at<? GROUP BY bucket_id", time.Now().UTC())
	if e != nil {
		return e
	}
	for rows.Next() {
		var bucketID string
		var released int64
		if e = rows.Scan(&bucketID, &released); e != nil {
			rows.Close()
			return e
		}
		if _, e = tx.ExecContext(ctx, "UPDATE buckets SET reserved_bytes=MAX(0,reserved_bytes-?) WHERE id=?", released, bucketID); e != nil {
			rows.Close()
			return e
		}
	}
	rows.Close()
	if _, e = tx.ExecContext(ctx, "DELETE FROM uploads WHERE expires_at<?", time.Now().UTC()); e != nil {
		return e
	}
	var pending int64
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM uploads WHERE bucket_id=?", o.BucketID).Scan(&pending); e != nil {
		return e
	}
	if pending >= maxPending {
		return errors.New("pending upload count limit exceeded")
	}
	var existing int
	if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM objects WHERE bucket_id=? AND logical_key=? AND status='ready')", o.BucketID, o.LogicalKey).Scan(&existing); e != nil {
		return e
	}
	if existing == 0 {
		var objectCount int64
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM objects WHERE bucket_id=? AND status='ready'", o.BucketID).Scan(&objectCount); e != nil {
			return e
		}
		if objectCount >= maxObjects {
			return errors.New("object count limit exceeded")
		}
	}

	var q, u, r, oldSize int64
	if e = tx.QueryRowContext(ctx, "SELECT quota_bytes,used_bytes,reserved_bytes FROM buckets WHERE id=?", o.BucketID).Scan(&q, &u, &r); e != nil {
		return e
	}
	_ = tx.QueryRowContext(ctx, "SELECT size FROM objects WHERE bucket_id=? AND logical_key=? AND status='ready'", o.BucketID, o.LogicalKey).Scan(&oldSize)
	reserved := o.Size - oldSize
	if reserved < 0 {
		reserved = 0
	}
	if u+r+reserved > q {
		return errors.New("bucket quota exceeded")
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO uploads(id,bucket_id,logical_key,physical_key,size,content_type,reserved_bytes,expires_at,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, o.ID, o.BucketID, o.LogicalKey, o.PhysicalKey, o.Size, o.ContentType, reserved, expiresAt, o.CreatedAt)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "UPDATE buckets SET reserved_bytes=reserved_bytes+? WHERE id=?", reserved, o.BucketID)
	if e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) GetUpload(ctx context.Context, bucketID, id, key string) (model.Object, error) {
	var o model.Object
	e := s.db.QueryRowContext(ctx, `SELECT id,bucket_id,logical_key,physical_key,size,content_type,created_at FROM uploads WHERE bucket_id=? AND id=? AND logical_key=?`, bucketID, id, key).Scan(&o.ID, &o.BucketID, &o.LogicalKey, &o.PhysicalKey, &o.Size, &o.ContentType, &o.CreatedAt)
	o.Status = "pending"
	return o, e
}

func (s *Store) CancelUpload(ctx context.Context, id string) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var bucketID string
	var reserved int64
	if e = tx.QueryRowContext(ctx, "SELECT bucket_id,reserved_bytes FROM uploads WHERE id=?", id).Scan(&bucketID, &reserved); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM uploads WHERE id=?", id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE buckets SET reserved_bytes=MAX(0,reserved_bytes-?) WHERE id=?", reserved, bucketID); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) GetObject(ctx context.Context, bucketID, key string) (model.Object, error) {
	var o model.Object
	e := s.db.QueryRowContext(ctx, "SELECT id,bucket_id,logical_key,physical_key,size,content_type,etag,status,generation,is_public,created_at,updated_at FROM objects WHERE bucket_id=? AND logical_key=?", bucketID, key).Scan(&o.ID, &o.BucketID, &o.LogicalKey, &o.PhysicalKey, &o.Size, &o.ContentType, &o.ETag, &o.Status, &o.Generation, &o.Public, &o.CreatedAt, &o.UpdatedAt)
	return o, e
}
func (s *Store) CommitUpload(ctx context.Context, upload model.Object, etag string, actual int64) (model.Object, string, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return model.Object{}, "", e
	}
	defer tx.Rollback()
	var reserved int64
	if e = tx.QueryRowContext(ctx, "SELECT reserved_bytes FROM uploads WHERE id=?", upload.ID).Scan(&reserved); e != nil {
		return model.Object{}, "", e
	}
	var old model.Object
	oldErr := tx.QueryRowContext(ctx, "SELECT id,physical_key,size,generation,created_at FROM objects WHERE bucket_id=? AND logical_key=?", upload.BucketID, upload.LogicalKey).Scan(&old.ID, &old.PhysicalKey, &old.Size, &old.Generation, &old.CreatedAt)
	if oldErr != nil && !errors.Is(oldErr, sql.ErrNoRows) {
		return model.Object{}, "", oldErr
	}
	objectID := old.ID
	createdAt := old.CreatedAt
	oldPhysical := old.PhysicalKey
	if objectID == "" {
		objectID = secretID(upload.ID)
		createdAt = upload.CreatedAt
	}
	now := time.Now().UTC()
	generation := old.Generation + 1
	if generation < 1 {
		generation = 1
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO objects(id,bucket_id,logical_key,physical_key,size,content_type,etag,status,generation,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(bucket_id,logical_key) DO UPDATE SET physical_key=excluded.physical_key,size=excluded.size,content_type=excluded.content_type,etag=excluded.etag,status='ready',generation=excluded.generation,updated_at=excluded.updated_at`, objectID, upload.BucketID, upload.LogicalKey, upload.PhysicalKey, actual, upload.ContentType, etag, "ready", generation, createdAt, now)
	if e != nil {
		return model.Object{}, "", e
	}
	_, e = tx.ExecContext(ctx, "UPDATE buckets SET reserved_bytes=MAX(0,reserved_bytes-?),used_bytes=MAX(0,used_bytes-?)+? WHERE id=?", reserved, old.Size, actual, upload.BucketID)
	if e != nil {
		return model.Object{}, "", e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM uploads WHERE id=?", upload.ID); e != nil {
		return model.Object{}, "", e
	}
	if e = tx.Commit(); e != nil {
		return model.Object{}, "", e
	}
	upload.ID, upload.Size, upload.ETag, upload.Status, upload.Generation, upload.CreatedAt, upload.UpdatedAt = objectID, actual, etag, "ready", generation, createdAt, now
	return upload, oldPhysical, nil
}

func secretID(uploadID string) string { return "obj_" + strings.TrimPrefix(uploadID, "obj_") }
func (s *Store) DeleteObject(ctx context.Context, id string) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var bid, status string
	var size int64
	if e = tx.QueryRowContext(ctx, "SELECT bucket_id,status,size FROM objects WHERE id=?", id).Scan(&bid, &status, &size); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM public_links WHERE object_id=?", id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM objects WHERE id=?", id); e != nil {
		return e
	}
	if status == "ready" {
		_, e = tx.ExecContext(ctx, "UPDATE buckets SET used_bytes=MAX(0,used_bytes-?) WHERE id=?", size, bid)
	} else {
		_, e = tx.ExecContext(ctx, "UPDATE buckets SET reserved_bytes=MAX(0,reserved_bytes-?) WHERE id=?", size, bid)
	}
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) ListObjects(ctx context.Context, bucketID, prefix string) ([]model.Object, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT id,bucket_id,logical_key,physical_key,size,content_type,etag,status,generation,is_public,created_at,updated_at FROM objects WHERE bucket_id=? AND status='ready' AND logical_key LIKE ? ORDER BY logical_key LIMIT 1000", bucketID, prefix+"%")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Object
	for rows.Next() {
		var o model.Object
		if e = rows.Scan(&o.ID, &o.BucketID, &o.LogicalKey, &o.PhysicalKey, &o.Size, &o.ContentType, &o.ETag, &o.Status, &o.Generation, &o.Public, &o.CreatedAt, &o.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (s *Store) CreatePublicLink(ctx context.Context, id, obj, slug string, ttl int64, expires *time.Time) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO public_links(id,object_id,slug,sign_ttl_seconds,expires_at,created_at) VALUES(?,?,?,?,?,?)", id, obj, slug, ttl, expires, time.Now().UTC())
	return e
}
func (s *Store) RevokePublicLink(ctx context.Context, bucketID, slug string) error {
	r, e := s.db.ExecContext(ctx, `UPDATE public_links SET revoked=1 WHERE slug=? AND object_id IN (SELECT id FROM objects WHERE bucket_id=?)`, slug, bucketID)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *Store) PublicObject(ctx context.Context, slug string) (model.Object, int64, error) {
	var o model.Object
	var ttl int64
	var expiry sql.NullTime
	e := s.db.QueryRowContext(ctx, `SELECT o.id,o.bucket_id,o.logical_key,o.physical_key,o.size,o.content_type,o.etag,o.status,o.generation,o.is_public,o.created_at,o.updated_at,l.sign_ttl_seconds,l.expires_at FROM public_links l JOIN objects o ON o.id=l.object_id WHERE l.slug=? AND l.revoked=0`, slug).Scan(&o.ID, &o.BucketID, &o.LogicalKey, &o.PhysicalKey, &o.Size, &o.ContentType, &o.ETag, &o.Status, &o.Generation, &o.Public, &o.CreatedAt, &o.UpdatedAt, &ttl, &expiry)
	if e == nil && expiry.Valid && time.Now().After(expiry.Time) {
		return o, ttl, errors.New("link expired")
	}
	return o, ttl, e
}
