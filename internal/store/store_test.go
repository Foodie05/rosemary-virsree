package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"rosemary-virsree/internal/model"
)

func TestObjectPagesAndFolderBrowse(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pages.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	b := model.Bucket{ID: "browse", Name: "Browse", Slug: "browse-bucket", Visibility: "private", QuotaBytes: 1 << 30, CreatedAt: time.Now().UTC()}
	if _, err = db.CreateBucket(ctx, b, 1<<30, false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1003; i++ {
		key := fmt.Sprintf("images/%04d.jpg", i)
		_, err = db.db.ExecContext(ctx, `INSERT INTO objects(id,bucket_id,logical_key,physical_key,size,content_type,etag,status,created_at,updated_at) VALUES(?,?,?,?,1,'text/plain','','ready',?,?)`, fmt.Sprintf("obj-%d", i), b.ID, key, fmt.Sprintf("physical-%d", i), time.Now(), time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.db.ExecContext(ctx, `INSERT INTO objects(id,bucket_id,logical_key,physical_key,size,content_type,etag,status,created_at,updated_at) VALUES('literal','browse','%literal.txt','physical-literal',1,'text/plain','','ready',?,?)`, time.Now(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	first, next, err := db.ListObjectsPage(ctx, b.ID, "images/", "", 1000)
	if err != nil || len(first) != 1000 || next != first[999].LogicalKey {
		t.Fatalf("first page: %d, next=%q, err=%v", len(first), next, err)
	}
	last, next, err := db.ListObjectsPage(ctx, b.ID, "images/", next, 1000)
	if err != nil || len(last) != 3 || next != "" {
		t.Fatalf("last page: %d, next=%q, err=%v", len(last), next, err)
	}
	literal, _, err := db.ListObjectsPage(ctx, b.ID, "%", "", 40)
	if err != nil || len(literal) != 1 || literal[0].LogicalKey != "%literal.txt" {
		t.Fatalf("literal prefix: %+v, err=%v", literal, err)
	}
	root, next, err := db.BrowseObjectsPage(ctx, b.ID, "", "", 1)
	if err != nil || len(root) != 1 || next == "" || root[0].Key != "%literal.txt" {
		t.Fatalf("root page: %+v, next=%q, err=%v", root, next, err)
	}
	root, next, err = db.BrowseObjectsPage(ctx, b.ID, "", next, 1)
	if err != nil || len(root) != 1 || root[0].Key != "images/" || !root[0].Folder || next != "" {
		t.Fatalf("folder page: %+v, next=%q, err=%v", root, next, err)
	}
	inside, _, err := db.BrowseObjectsPage(ctx, b.ID, "images/", "", 1)
	if err != nil || len(inside) != 1 || inside[0].Object == nil || inside[0].Object.LogicalKey != "images/0000.jpg" {
		t.Fatalf("folder contents: %+v, err=%v", inside, err)
	}
}

func TestReplacementOnlyReservesGrowth(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "quota.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	b := model.Bucket{ID: "b1", Name: "Bucket", Slug: "bucket-one", Visibility: "private", QuotaBytes: 200, CreatedAt: time.Now().UTC()}
	if _, err = db.CreateBucket(ctx, b, 1000, false); err != nil {
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
	if _, _, err = db.PublicObject(ctx, "public-one"); err == nil {
		t.Fatal("private virtual bucket exposed a public alias")
	}
	if _, err = db.UpdateBucket(ctx, b.Slug, b.Name, "public", b.QuotaBytes, false, 1000, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.PublicObject(ctx, "public-one"); err != nil {
		t.Fatalf("public alias should resolve in public mode: %v", err)
	}
	if _, err = db.UpdateBucket(ctx, b.Slug, b.Name, "private", b.QuotaBytes, false, 1000, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.PublicObject(ctx, "public-one"); err == nil {
		t.Fatal("public alias survived switch to private mode")
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

func TestBucketQuotaCanBeEditedAndUnlimitedIsExplicit(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "quota-edit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	b := model.Bucket{ID: "b2", Name: "Editable", Slug: "editable-bucket", Visibility: "private", QuotaBytes: 100, CreatedAt: time.Now().UTC()}
	if _, err = db.CreateBucket(ctx, b, 1000, false); err != nil {
		t.Fatal(err)
	}
	if _, err = db.UpdateBucket(ctx, b.Slug, "Bigger", "public", 250, false, 1000, false); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetBucket(ctx, b.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if got.QuotaBytes != 250 || got.Visibility != "public" || got.QuotaUnlimited {
		t.Fatalf("unexpected updated bucket: %#v", got)
	}
	if _, err = db.UpdateBucket(ctx, b.Slug, "Unlimited", "private", 0, true, 1000, false); err == nil {
		t.Fatal("unlimited bucket accepted on finite primary source")
	}
	got, err = db.UpdateBucket(ctx, b.Slug, "Unlimited", "private", 0, true, 1000, true)
	if err != nil {
		t.Fatal(err)
	}
	if !got.QuotaUnlimited {
		t.Fatal("unlimited flag was not persisted")
	}
}
