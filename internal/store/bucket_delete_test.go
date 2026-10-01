package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rosemary-virsree/internal/model"
)

func TestBucketDeletionWaitCleanupAndIsolation(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "deletion.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	for _, id := range []string{"target", "other"} {
		_, err = db.CreateBucket(ctx, model.Bucket{ID: id, Slug: id, Name: id, Visibility: "public", QuotaBytes: 1000, CreatedAt: now}, 10000, false)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.CreateStorageSource(ctx, model.StorageSource{ID: id, Name: id, Kind: "s3", CapacityBytes: 10000, Enabled: true, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		u := model.Object{ID: id + "-upload", BucketID: id, SourceID: id, LogicalKey: "file", PhysicalKey: "rosemary/" + id + "/g1/file", Size: 100, CreatedAt: now}
		if err = db.ReserveObject(ctx, u, now.Add(-time.Hour), 100, 10); err != nil {
			t.Fatal(err)
		}
		o, _, _, e := db.CommitUpload(ctx, u, "etag", 100)
		if e != nil {
			t.Fatal(e)
		}
		if err = db.CreatePublicLink(ctx, id+"-link", o.ID, id+"-public", 60, nil); err != nil {
			t.Fatal(err)
		}
		if err = db.CreateDownloadToken(ctx, id+"-token", o.ID, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if err = db.CreateAccessKey(ctx, model.AccessKey{ID: id + "-key", BucketID: id, AK: id + "-ak", CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		u.ID = id + "-pending"
		u.PhysicalKey = "rosemary-staging/" + id + "/pending/file"
		u.LogicalKey = "pending"
		u.Size = 50
		if err = db.ReserveObject(ctx, u, now.Add(-time.Hour), 100, 10); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.PrepareBucketDeletion(ctx, "target", "challenge", now.Add(time.Hour), now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = db.BeginBucketDeletion(ctx, "target", "challenge"); err == nil || !strings.Contains(err.Error(), "wait required") {
		t.Fatalf("must enforce wait: %v", err)
	}
	if err = db.BeginBucketDeletion(ctx, "other", "challenge"); err == nil {
		t.Fatal("challenge was allowed for another bucket")
	}
	if _, err = db.db.Exec(`UPDATE bucket_delete_confirmations SET ready_at=?`, now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = db.BeginBucketDeletion(ctx, "target", "challenge"); err != nil {
		t.Fatal(err)
	}
	b, err := db.GetBucket(ctx, "target")
	if err != nil || !b.Deleting {
		t.Fatalf("deletion state missing: %+v %v", b, err)
	}
	if _, _, err = db.AccessByAK(ctx, "target-ak"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("key still authenticates: %v", err)
	}
	if _, _, err = db.PublicObject(ctx, "target-public"); err == nil {
		t.Fatal("public alias still available")
	}
	if _, _, err = db.DownloadByTransferHash(ctx, "target-token"); err == nil {
		t.Fatal("download token still available")
	}
	u := model.Object{ID: "late", BucketID: "target", SourceID: "target", LogicalKey: "late", PhysicalKey: "late", Size: 1, CreatedAt: now}
	if err = db.ReserveObject(ctx, u, now.Add(time.Hour), 100, 10); !errors.Is(err, ErrBucketDeleting) {
		t.Fatalf("new upload allowed: %v", err)
	}
	if err = db.CreateAccessKey(ctx, model.AccessKey{ID: "late", BucketID: "target", AK: "late", CreatedAt: now}); !errors.Is(err, ErrBucketDeleting) {
		t.Fatalf("new key allowed: %v", err)
	}
	if _, err = db.UpdateBucket(ctx, "target", "changed", "private", 1000, false, 10000, false); !errors.Is(err, ErrBucketDeleting) {
		t.Fatalf("edit allowed: %v", err)
	}
	u.ID = "target-pending"
	if _, _, _, err = db.CommitUpload(ctx, u, "etag", 1); !errors.Is(err, ErrBucketDeleting) {
		t.Fatalf("commit allowed: %v", err)
	}
	if err = db.FinishBucketDeletion(ctx, "target"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"buckets", "objects", "uploads", "upload_grants", "access_keys", "public_links", "transfer_tokens", "bucket_deletions", "bucket_delete_confirmations"} {
		var count int
		if err = db.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		want := 1
		if table == "objects" || table == "uploads" {
			want = 1
		}
		if table == "upload_grants" {
			want = 2
		}
		if table == "bucket_deletions" || table == "bucket_delete_confirmations" {
			want = 0
		}
		if count != want {
			t.Fatalf("%s count=%d want=%d", table, count, want)
		}
	}
	target, _ := db.GetStorageSource(ctx, "target")
	other, _ := db.GetStorageSource(ctx, "other")
	if target.UsedBytes != 0 || target.ReservedBytes != 0 || other.UsedBytes != 100 || other.ReservedBytes != 50 {
		t.Fatalf("incorrect accounting target=%+v other=%+v", target, other)
	}
	if err = db.FinishBucketDeletion(ctx, "target"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("repeat cleanup: %v", err)
	}
}

func TestCommittedPutGrantDelaysDeletionAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "grants.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	expires := now.Add(time.Hour)
	_, err = db.CreateBucket(ctx, model.Bucket{ID: "b", Slug: "bucket", Name: "Bucket", Visibility: "private", QuotaBytes: 1000, CreatedAt: now}, 1000, false)
	if err != nil {
		t.Fatal(err)
	}
	u := model.Object{ID: "u", BucketID: "b", LogicalKey: "file", PhysicalKey: "rosemary/b/g1/file", Size: 10, CreatedAt: now}
	if err = db.ReserveObject(ctx, u, expires, 100, 10); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = db.CommitUpload(ctx, u, "etag", 10); err != nil {
		t.Fatal(err)
	}
	if err = db.PrepareBucketDeletion(ctx, "b", "ok", now.Add(-time.Second), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = db.BeginBucketDeletion(ctx, "b", "ok"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	d, err := db.BucketDeletion(ctx, "b")
	if err != nil || !d.CleanupAfter.Equal(expires.Add(time.Minute)) {
		t.Fatalf("grant lost: %+v %v", d, err)
	}
	if err = db.FinishBucketDeletion(ctx, "b"); err == nil {
		t.Fatal("deleted metadata while PUT grant was live")
	}
	if err = db.BeginBucketDeletion(ctx, "b", "ok"); err != nil {
		t.Fatalf("retry not idempotent: %v", err)
	}
}

func TestUpgradeProtectsLegacyCommittedDirectPutsOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = db.CreateBucket(ctx, model.Bucket{ID: "legacy", Slug: "legacy-bucket", Name: "Legacy", Visibility: "private", QuotaBytes: 1000, CreatedAt: now}, 1000, false)
	if err != nil {
		t.Fatal(err)
	}
	db.Audit(ctx, "upload.signed", "legacy-bucket", "no expiry in old audit")
	// Simulate the pre-receipt schema. No upload remains after a legacy commit.
	if _, err = db.db.Exec(`DROP TABLE upload_grants`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var expiry time.Time
	if err = db.db.QueryRow(`SELECT expires_at FROM upload_grants WHERE id='legacy_legacy'`).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	if expiry.Before(now.Add(7 * 24 * time.Hour)) {
		t.Fatal("legacy direct PUT protection missing")
	}
	// A newer receipt-based audit must not extend the legacy deadline on restart.
	db.Audit(ctx, "upload.signed", "legacy-bucket", "new signature")
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var after time.Time
	if err = db.db.QueryRow(`SELECT expires_at FROM upload_grants WHERE id='legacy_legacy'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(expiry) {
		t.Fatal("restart extended legacy grant protection")
	}
}
