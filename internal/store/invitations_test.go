package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"rosemary-virsree/internal/model"
)

func invitationFixture(t *testing.T) (*Store, model.Bucket, UploadInvitation) {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "invitation.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// Serialize SQLite writers so concurrent policy failures return stable codes.
	db.db.SetMaxOpenConns(1)
	ctx := context.Background()
	b := model.Bucket{ID: "bucket", Slug: "bucket", Name: "Bucket", Visibility: "public", QuotaBytes: 1000, CreatedAt: time.Now().UTC()}
	if _, err = db.CreateBucket(ctx, b, 1000, false); err != nil {
		t.Fatal(err)
	}
	i := UploadInvitation{ID: "invitation", BucketID: b.ID, Name: "Invitation", TokenHash: "test-hash-not-a-token", MaxFiles: 1, ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC()}
	if err = db.CreateUploadInvitation(ctx, i); err != nil {
		t.Fatal(err)
	}
	return db, b, i
}
func TestInvitationLimitIsAtomicAndExpiredReservationsFreeSlots(t *testing.T) {
	db, b, i := invitationFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for n := 0; n < 10; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('a' + n))
			o := model.Object{ID: key, BucketID: b.ID, LogicalKey: key, PhysicalKey: key, Size: 1, CreatedAt: time.Now().UTC()}
			err := db.ReserveInvitationObject(ctx, o, time.Now().Add(time.Minute), 100, 100, i.ID)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				success++
			} else if !errors.Is(err, ErrInvitationLimit) {
				t.Errorf("unexpected reservation failure: %v", err)
			}
		}(n)
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("reservations admitted: %d", success)
	}
	if _, err := db.db.Exec(`UPDATE uploads SET expires_at=?`, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	o := model.Object{ID: "next", BucketID: b.ID, LogicalKey: "next", PhysicalKey: "next", Size: 1, CreatedAt: time.Now().UTC()}
	if err := db.ReserveInvitationObject(ctx, o, time.Now().Add(time.Minute), 100, 100, i.ID); err != nil {
		t.Fatal(err)
	}
}
func TestInvitationCannotCommitAfterRevocationAndCannotOverwriteAtCommit(t *testing.T) {
	db, b, i := invitationFixture(t)
	ctx := context.Background()
	o := model.Object{ID: "upload", BucketID: b.ID, LogicalKey: "report", PhysicalKey: "staging", Size: 4, CreatedAt: time.Now().UTC()}
	if err := db.ReserveInvitationObject(ctx, o, time.Now().Add(time.Minute), 100, 100, i.ID); err != nil {
		t.Fatal(err)
	}
	// An owner publishes the same key after the invite's reservation.
	_, err := db.db.Exec(`INSERT INTO objects(id,bucket_id,logical_key,physical_key,size,content_type,etag,status,created_at,updated_at) VALUES('owner',?,'report','owner-physical',1,'text/plain','','ready',?,?)`, b.ID, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = db.CommitUpload(ctx, o, "etag", 4); !errors.Is(err, ErrInvitationOverwrite) {
		t.Fatalf("overwrite was allowed: %v", err)
	}
	if err = db.RevokeUploadInvitation(ctx, b.ID, i.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = db.CommitUpload(ctx, o, "etag", 4); !errors.Is(err, ErrInvitationInactive) {
		t.Fatalf("revoked commit: %v", err)
	}
}
func TestInvitationReceiptAndPublicAliasSurviveReopenAndBucketDeletion(t *testing.T) {
	db, b, i := invitationFixture(t)
	ctx := context.Background()
	o := model.Object{ID: "upload", BucketID: b.ID, LogicalKey: "report", PhysicalKey: "final", Size: 4, CreatedAt: time.Now().UTC()}
	if err := db.ReserveInvitationObject(ctx, o, time.Now().UTC().Add(-2*time.Minute), 100, 100, i.ID); err != nil {
		t.Fatal(err)
	}
	ready, _, _, err := db.CommitUpload(ctx, o, "etag", 4)
	if err != nil {
		t.Fatal(err)
	}
	r, err := db.InvitationUpload(ctx, i.ID, o.ID, o.LogicalKey)
	if err != nil || r.Status != "committed" || r.ObjectID != ready.ID || r.PublicSlug == "" {
		t.Fatalf("receipt: %+v %v", r, err)
	}
	var ttl int
	var expiry any
	if err = db.db.QueryRow(`SELECT sign_ttl_seconds,expires_at FROM public_links WHERE slug=?`, r.PublicSlug).Scan(&ttl, &expiry); err != nil || ttl != 600 || expiry != nil {
		t.Fatalf("public alias expiry: %v %v", expiry, err)
	}
	// Migration on a database with the production schema must retain receipts.
	if err = db.migrate(); err != nil {
		t.Fatal(err)
	}
	same, err := db.InvitationUpload(ctx, i.ID, o.ID, o.LogicalKey)
	if err != nil || same != r {
		t.Fatalf("migration changed receipt: %+v %v", same, err)
	}
	if err = db.PrepareBucketDeletion(ctx, b.ID, "confirm", time.Now().UTC().Add(-time.Second), time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = db.BeginBucketDeletion(ctx, b.ID, "confirm"); err != nil {
		t.Fatal(err)
	}
	if err = db.FinishBucketDeletion(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"upload_invitations", "invitation_uploads", "public_links", "buckets"} {
		var count int
		if err = db.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("cleanup %s: %d %v", table, count, err)
		}
	}
}
