package service

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rosemary-virsree/internal/config"
	"rosemary-virsree/internal/model"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/store"
)

type failingCleanupBackend struct {
	memoryBackend
	fail bool
}

func (m *failingCleanupBackend) PurgePrefix(ctx context.Context, prefix string) error {
	if m.fail {
		return errors.New("provider sensitive error with credentials")
	}
	return m.memoryBackend.PurgePrefix(ctx, prefix)
}

func TestBucketCleanupRetriesAndPurgesOrphansAcrossSources(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	box, _ := secretbox.New("test-master-key")
	manager := NewStorageManager(db, box, "https://example.test", nil)
	a := &failingCleanupBackend{memoryBackend: memoryBackend{objects: map[string][]byte{}}}
	b := &failingCleanupBackend{memoryBackend: memoryBackend{objects: map[string][]byte{}}, fail: true}
	now := time.Now().UTC()
	for id, backend := range map[string]*failingCleanupBackend{"a": a, "b": b} {
		meta := model.StorageSource{ID: id, Name: id, Kind: "s3", CapacityBytes: 10000, Enabled: true, CreatedAt: now}
		if err = db.CreateStorageSource(ctx, meta); err != nil {
			t.Fatal(err)
		}
		manager.sources[id] = sourceRuntime{meta: meta, backend: backend}
	}
	svc := &Service{DB: db, Storage: manager, Box: box, Config: config.Config{TotalQuota: 1000, MaxObjectsPerBucket: 100, MaxPendingUploads: 10}}
	bucket, err := svc.NewBucket(ctx, "Target", "target-bucket", "private", 1000, false)
	if err != nil {
		t.Fatal(err)
	}
	key, secret, err := svc.NewKey(ctx, bucket, "owner", "read,write,delete,manage")
	if err != nil {
		t.Fatal(err)
	}
	for id, backend := range map[string]*failingCleanupBackend{"a": a, "b": b} {
		key := physical(bucket, "file-"+id, 1)
		u := model.Object{ID: "upload-" + id, BucketID: bucket.ID, SourceID: id, LogicalKey: "file-" + id, PhysicalKey: key, Size: 5, CreatedAt: now}
		if err = db.ReserveObject(ctx, u, now.Add(-time.Hour), 100, 10); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err = db.CommitUpload(ctx, u, "etag", 5); err != nil {
			t.Fatal(err)
		}
		backend.objects[key] = []byte("value")
		backend.objects["rosemary-staging/"+bucket.ID+"/old-untracked/file"] = []byte("orphan")
		backend.objects["rosemary/other/g1/keep"] = []byte("other")
		backend.objects["virsree-system/v1/backup"] = []byte("backup")
	}
	if err = svc.DeleteBucket(ctx, bucket.Slug, bucket.Slug+" ", "ignored"); err == nil {
		t.Fatal("name whitespace accepted")
	}
	if err = svc.DeleteBucket(ctx, bucket.Slug, strings.ToUpper(bucket.Slug), "ignored"); err == nil {
		t.Fatal("name case accepted")
	}
	token, ready, err := svc.PrepareBucketDeletion(ctx, bucket.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(ready) < 4*time.Second {
		t.Fatal("confirmation wait was not five seconds")
	}
	if err = svc.DeleteBucket(ctx, bucket.Slug, bucket.Slug, token); err == nil {
		t.Fatal("server wait bypassed")
	}
	if err = db.PrepareBucketDeletion(ctx, bucket.ID, secretbox.Hash("ready"), now.Add(-time.Second), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = svc.DeleteBucket(ctx, bucket.Slug, bucket.Slug, "ready"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Authenticate(ctx, key.AK, secret, "read"); err == nil {
		t.Fatal("credential still authenticates")
	}
	stale := Credential{Bucket: bucket}
	if _, _, err = svc.BeginUpload(ctx, stale, "late", "text/plain", 1, 60); !errors.Is(err, store.ErrBucketDeleting) {
		t.Fatalf("stale credential wrote: %v", err)
	}
	svc.cleanupBuckets(ctx)
	if _, err = db.GetBucket(ctx, bucket.Slug); err != nil {
		t.Fatal("failure discarded bucket manifest")
	}
	d, err := db.BucketDeletion(ctx, bucket.ID)
	if err != nil || d.LastErrorCode != "bucket_cleanup_failed" {
		t.Fatalf("missing stable failure code: %+v %v", d, err)
	}
	b.fail = false
	svc.cleanupBuckets(ctx)
	if _, err = db.GetBucket(ctx, bucket.Slug); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cleanup not finished: %v", err)
	}
	for _, backend := range []*failingCleanupBackend{a, b} {
		if len(backend.objects) != 2 {
			t.Fatalf("orphan remained: %+v", backend.objects)
		}
		if string(backend.objects["rosemary/other/g1/keep"]) != "other" || string(backend.objects["virsree-system/v1/backup"]) != "backup" {
			t.Fatal("unrelated files removed")
		}
	}
	for _, id := range []string{"a", "b"} {
		source, _ := db.GetStorageSource(ctx, id)
		if source.UsedBytes != 0 || source.ReservedBytes != 0 {
			t.Fatalf("capacity not released: %+v", source)
		}
	}
	svc.cleanupBuckets(ctx) // Completed work does not run again or alter accounting.
	// A PUT started before expiry may finish after the bucket metadata is gone.
	a.objects["rosemary-staging/"+bucket.ID+"/late-completion/file"] = []byte("late")
	if err = svc.sweepBucketNamespaces(ctx, bucket.ID, true); err != nil {
		t.Fatal(err)
	}
	if len(a.objects) != 2 || string(a.objects["rosemary/other/g1/keep"]) != "other" {
		t.Fatal("late-transfer cleanup failed or affected another bucket")
	}
}

func TestDeletionAcceptsImmediatelyButCleanupWaitsForInflightOperation(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "inflight.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	box, _ := secretbox.New("inflight-test")
	svc := &Service{DB: db, Box: box, Storage: NewStorageManager(db, box, "", nil), Config: config.Config{TotalQuota: 1000}}
	b, err := svc.NewBucket(ctx, "Bucket", "bucket", "private", 1000, false)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err = db.PrepareBucketDeletion(ctx, b.ID, secretbox.Hash("ready"), now.Add(-time.Second), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	unlock, err := svc.BucketOperation(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	deleting := make(chan error, 1)
	go func() { deleting <- svc.DeleteBucket(ctx, b.Slug, b.Slug, "ready") }()
	select {
	case err = <-deleting:
		if err != nil {
			unlock()
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		unlock()
		t.Fatal("deletion blocked on in-flight transfer")
	}
	if _, err = svc.BucketOperation(ctx, b.ID); !errors.Is(err, store.ErrBucketDeleting) {
		unlock()
		t.Fatalf("new operations not rejected immediately: %v", err)
	}
	cleaned := make(chan struct{})
	go func() { svc.cleanupBuckets(ctx); close(cleaned) }()
	select {
	case <-cleaned:
		unlock()
		t.Fatal("cleanup raced an in-flight operation")
	case <-time.After(50 * time.Millisecond):
	}
	unlock()
	select {
	case <-cleaned:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not resume")
	}
	if _, err = db.GetBucket(ctx, b.Slug); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("bucket remains after transfer finished: %v", err)
	}
}
