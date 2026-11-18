package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"rosemary-virsree/internal/config"
	"rosemary-virsree/internal/model"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/store"
)

func TestInvitedUploadIsIdempotentAndPrivateBucketsHaveNoPublicURL(t *testing.T) {
	for _, visibility := range []string{"public", "private"} {
		t.Run(visibility, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(filepath.Join(t.TempDir(), "invitation.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			box, _ := secretbox.New("test-key")
			manager := NewStorageManager(db, box, "https://gateway.example", nil)
			backend := &memoryBackend{objects: map[string][]byte{}}
			meta := model.StorageSource{ID: "source", Name: "Source", Kind: "s3", CapacityBytes: 10000, Enabled: true, Direct: true, CreatedAt: time.Now().UTC()}
			if err = db.CreateStorageSource(ctx, meta); err != nil {
				t.Fatal(err)
			}
			manager.sources[meta.ID] = sourceRuntime{meta: meta, backend: backend}
			svc := &Service{DB: db, Storage: manager, Box: box, Config: config.Config{PublicURL: "https://gateway.example", TotalQuota: 10000, MaxObjectsPerBucket: 100, MaxPendingUploads: 10}}
			bucket, err := svc.NewBucket(ctx, "Fixture", "fixture-bucket", visibility, 1000, false)
			if err != nil {
				t.Fatal(err)
			}
			i, token, err := svc.CreateUploadInvitation(ctx, bucket, "Invitation", 1, 3600, false)
			if err != nil {
				t.Fatal(err)
			}
			_, c, err := svc.AuthenticateUploadInvitation(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			o, _, expires, err := svc.BeginInvitedUpload(ctx, i, c, "report.txt", "text/plain", 4)
			if err != nil || expires > 900 || expires <= 0 {
				t.Fatalf("begin: %v", err)
			}
			backend.objects[o.PhysicalKey] = []byte("test")
			result, url, err := svc.CommitInvitedUpload(ctx, i, c, o.ID, o.LogicalKey)
			if err != nil {
				t.Fatal(err)
			}
			repeated, repeatedURL, err := svc.CommitInvitedUpload(ctx, i, c, o.ID, o.LogicalKey)
			if err != nil || result.ID != repeated.ID || url != repeatedURL {
				t.Fatalf("idempotency: %v", err)
			}
			if (visibility == "private") != (url == "") {
				t.Fatal("incorrect public URL policy")
			}
			current, _ := db.UploadInvitation(ctx, i.ID)
			if current.UsedFiles != 1 || current.PendingFiles != 0 {
				t.Fatalf("slot count: %+v", current)
			}
			if _, _, _, err = svc.BeginInvitedUpload(ctx, i, c, "another", "text/plain", 4); !errors.Is(err, store.ErrInvitationLimit) {
				t.Fatalf("limit: %v", err)
			}
			if _, _, err = svc.CommitInvitedUpload(ctx, i, c, "other-id", o.LogicalKey); !errors.Is(err, store.ErrInvitationOwnership) {
				t.Fatal("foreign upload accepted")
			}
			if err = db.RevokeUploadInvitation(ctx, bucket.ID, i.ID); err != nil {
				t.Fatal(err)
			}
			if _, _, err = svc.CommitInvitedUpload(ctx, i, c, o.ID, o.LogicalKey); !errors.Is(err, store.ErrInvitationInactive) {
				t.Fatal("revoked receipt accepted")
			}
		})
	}
}
