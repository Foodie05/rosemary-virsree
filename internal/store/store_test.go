package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"rosemary-virsree/internal/model"
)

func TestReplacementOnlyReservesGrowth(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "quota.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	b := model.Bucket{ID: "b1", Name: "Bucket", Slug: "bucket-one", Visibility: "private", QuotaBytes: 200, CreatedAt: time.Now().UTC()}
	if _, err = db.CreateBucket(ctx, b, 1000); err != nil {
		t.Fatal(err)
	}

	first := model.Object{ID: "upload-1", BucketID: b.ID, LogicalKey: "same.txt", PhysicalKey: "p1", Size: 100, ContentType: "text/plain", CreatedAt: time.Now().UTC()}
	if err = db.ReserveObject(ctx, first, time.Now().Add(time.Minute), 100, 10); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = db.CommitUpload(ctx, first, "etag-1", 100); err != nil {
		t.Fatal(err)
	}

	replacement := model.Object{ID: "upload-2", BucketID: b.ID, LogicalKey: "same.txt", PhysicalKey: "p2", Size: 150, ContentType: "text/plain", CreatedAt: time.Now().UTC()}
	if err = db.ReserveObject(ctx, replacement, time.Now().Add(time.Minute), 100, 10); err != nil {
		t.Fatal(err)
	}
	buckets, err := db.ListBuckets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if buckets[0].UsedBytes != 100 || buckets[0].ReservedBytes != 50 {
		t.Fatalf("used/reserved = %d/%d, want 100/50", buckets[0].UsedBytes, buckets[0].ReservedBytes)
	}

	ready, oldPhysical, _, err := db.CommitUpload(ctx, replacement, "etag-2", 150)
	if err != nil {
		t.Fatal(err)
	}
	if oldPhysical != "p1" || ready.PhysicalKey != "p2" {
		t.Fatalf("physical replacement = %q -> %q", oldPhysical, ready.PhysicalKey)
	}
	buckets, _ = db.ListBuckets(ctx)
	if buckets[0].UsedBytes != 150 || buckets[0].ReservedBytes != 0 {
		t.Fatalf("used/reserved = %d/%d, want 150/0", buckets[0].UsedBytes, buckets[0].ReservedBytes)
	}
	if err = db.CreatePublicLink(ctx, "link-1", ready.ID, "public-one", 60, nil); err != nil {
		t.Fatal(err)
	}
	if err = db.RevokePublicLink(ctx, b.ID, "public-one"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.PublicObject(ctx, "public-one"); err == nil {
		t.Fatal("revoked public link still resolves")
	}
	if err = db.CreatePublicLink(ctx, "link-2", ready.ID, "public-two", 60, nil); err != nil {
		t.Fatal(err)
	}
	if err = db.DeleteObject(ctx, ready.ID); err != nil {
		t.Fatalf("delete object with public link: %v", err)
	}
	if _, _, err = db.PublicObject(ctx, "public-two"); err == nil {
		t.Fatal("public link survived object deletion")
	}
}
