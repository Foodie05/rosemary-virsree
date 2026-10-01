package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrBucketDeleting = errors.New("bucket deletion in progress")

type BucketDeletion struct {
	BucketID      string    `json:"-"`
	RequestedAt   time.Time `json:"requested_at"`
	CleanupAfter  time.Time `json:"cleanup_after"`
	LastErrorCode string    `json:"last_error_code"`
}

type querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requireActiveBucket(ctx context.Context, q querier, id string) error {
	var deleting bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bucket_deletions WHERE bucket_id=?) FROM buckets WHERE id=?`, id, id).Scan(&deleting); err != nil {
		return err
	}
	if deleting {
		return ErrBucketDeleting
	}
	return nil
}
func (s *Store) RequireActiveBucket(ctx context.Context, id string) error {
	return requireActiveBucket(ctx, s.db, id)
}

// Keep the deletion intent and all mappings until every provider has been cleaned.
// A valid direct PUT can recreate staging after commit, so receipts outlive uploads.
func (s *Store) BeginBucketDeletion(ctx context.Context, id, tokenHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	after := now
	var existing bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bucket_deletions WHERE bucket_id=?)`, id).Scan(&existing); err != nil {
		return err
	}
	if existing {
		return nil
	}
	var ready, expires time.Time
	if err = tx.QueryRowContext(ctx, `SELECT ready_at,expires_at FROM bucket_delete_confirmations WHERE bucket_id=? AND token_hash=?`, id, tokenHash).Scan(&ready, &expires); err != nil {
		return errors.New("bucket deletion confirmation invalid")
	}
	if now.Before(ready) {
		return errors.New("bucket deletion confirmation wait required")
	}
	if !now.Before(expires) {
		return errors.New("bucket deletion confirmation expired")
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM bucket_delete_confirmations WHERE bucket_id=?`, id); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, `SELECT expires_at FROM upload_grants WHERE bucket_id=?`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var exp time.Time
		if err = rows.Scan(&exp); err != nil {
			rows.Close()
			return err
		}
		if exp.Add(time.Minute).After(after) {
			after = exp.Add(time.Minute)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO bucket_deletions(bucket_id,requested_at,cleanup_after) VALUES(?,?,?)`, id, now, after); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE access_keys SET revoked=1 WHERE bucket_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE public_links SET revoked=1 WHERE object_id IN(SELECT id FROM objects WHERE bucket_id=?)`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) BucketDeletion(ctx context.Context, id string) (BucketDeletion, error) {
	var d BucketDeletion
	err := s.db.QueryRowContext(ctx, `SELECT bucket_id,requested_at,cleanup_after,last_error_code FROM bucket_deletions WHERE bucket_id=?`, id).Scan(&d.BucketID, &d.RequestedAt, &d.CleanupAfter, &d.LastErrorCode)
	return d, err
}
func (s *Store) PendingBucketDeletions(ctx context.Context) ([]BucketDeletion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT bucket_id,requested_at,cleanup_after,last_error_code FROM bucket_deletions ORDER BY requested_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BucketDeletion
	for rows.Next() {
		var d BucketDeletion
		if err = rows.Scan(&d.BucketID, &d.RequestedAt, &d.CleanupAfter, &d.LastErrorCode); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Store) BucketDeletionError(ctx context.Context, id, code string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE bucket_deletions SET last_error_code=? WHERE bucket_id=?`, code, id)
	return err
}

// Commit the metadata cleanup and release each source's accounting exactly once.
func (s *Store) FinishBucketDeletion(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var after time.Time
	if err = tx.QueryRowContext(ctx, `SELECT cleanup_after FROM bucket_deletions WHERE bucket_id=?`, id).Scan(&after); err != nil {
		return err
	}
	if time.Now().Before(after) {
		return errors.New("bucket upload grants have not expired")
	}
	// S3 checks the PUT signature when a request starts, not when its body ends.
	// Retain only the opaque namespace ID for a week of late-transfer sweeps.
	now := time.Now().UTC()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO bucket_cleanup_tombstones(bucket_id,until_at,next_at) VALUES(?,?,?)`, id, now.Add(7*24*time.Hour), now.Add(15*time.Minute)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE storage_sources SET
	 used_bytes=MAX(0,used_bytes-COALESCE((SELECT sum(size) FROM objects WHERE bucket_id=? AND source_id=storage_sources.id AND status='ready'),0)),
	 reserved_bytes=MAX(0,reserved_bytes-COALESCE((SELECT sum(source_reserved_bytes) FROM uploads WHERE bucket_id=? AND source_id=storage_sources.id),0))`, id, id); err != nil {
		return err
	}
	queries := []string{
		`DELETE FROM public_links WHERE object_id IN(SELECT id FROM objects WHERE bucket_id=?)`,
		`DELETE FROM transfer_tokens WHERE object_id IN(SELECT id FROM objects WHERE bucket_id=?)`,
		`DELETE FROM bucket_delete_confirmations WHERE bucket_id=?`, `DELETE FROM access_keys WHERE bucket_id=?`, `DELETE FROM uploads WHERE bucket_id=?`,
		`DELETE FROM upload_grants WHERE bucket_id=?`, `DELETE FROM objects WHERE bucket_id=?`,
		`DELETE FROM bucket_deletions WHERE bucket_id=?`, `DELETE FROM buckets WHERE id=?`,
	}
	for _, q := range queries {
		if _, err = tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type BucketCleanupTombstone struct {
	BucketID string
	Until    time.Time
}

func (s *Store) DueBucketCleanupTombstones(ctx context.Context) ([]BucketCleanupTombstone, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT bucket_id,until_at FROM bucket_cleanup_tombstones WHERE next_at<=? ORDER BY next_at LIMIT 50`, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BucketCleanupTombstone
	for rows.Next() {
		var v BucketCleanupTombstone
		if err = rows.Scan(&v.BucketID, &v.Until); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) UpdateBucketCleanupTombstone(ctx context.Context, id string, finished bool) error {
	if finished {
		_, err := s.db.ExecContext(ctx, `DELETE FROM bucket_cleanup_tombstones WHERE bucket_id=?`, id)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE bucket_cleanup_tombstones SET next_at=? WHERE bucket_id=?`, time.Now().UTC().Add(15*time.Minute), id)
	return err
}

func (s *Store) PrepareBucketDeletion(ctx context.Context, id, hash string, ready, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM bucket_delete_confirmations WHERE expires_at<?`, time.Now().UTC())
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO bucket_delete_confirmations(token_hash,bucket_id,ready_at,expires_at) VALUES(?,?,?,?)`, hash, id, ready, expires)
	return err
}

func (s *Store) PruneUploadGrants(ctx context.Context) {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM upload_grants WHERE expires_at<? AND NOT EXISTS(SELECT 1 FROM bucket_deletions d WHERE d.bucket_id=upload_grants.bucket_id)`, time.Now().UTC().Add(-time.Minute))
}

func (s *Store) ForgetUploadGrant(ctx context.Context, id string) {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM upload_grants WHERE id=?`, id)
}

func (s *Store) BucketPhysicalSources(ctx context.Context, id string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_id FROM objects WHERE bucket_id=? UNION SELECT source_id FROM uploads WHERE bucket_id=? UNION SELECT t.source_id FROM transfer_tokens t JOIN objects o ON o.id=t.object_id WHERE o.bucket_id=?`, id, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var source string
		if err = rows.Scan(&source); err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}
