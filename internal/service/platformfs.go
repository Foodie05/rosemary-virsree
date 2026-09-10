package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/store"
)

const platformFSPrefix = "virsree-system/v1"

type PlatformFilesystemStatus struct {
	Initialized       bool       `json:"initialized"`
	SourceID          string     `json:"source_id,omitempty"`
	SourceName        string     `json:"source_name,omitempty"`
	SourceKind        string     `json:"source_kind,omitempty"`
	Prefix            string     `json:"prefix"`
	Encryption        string     `json:"encryption"`
	BackupIntervalSec int64      `json:"backup_interval_seconds"`
	Retention         int        `json:"retention"`
	LastAttemptAt     *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt     *time.Time `json:"last_success_at,omitempty"`
	NextAttemptAt     *time.Time `json:"next_attempt_at,omitempty"`
	LastErrorCode     string     `json:"last_error_code,omitempty"`
}

type PlatformFilesystem struct {
	db        *store.Store
	storage   *StorageManager
	box       *secretbox.Box
	interval  time.Duration
	retention int
	mu        sync.RWMutex
	status    PlatformFilesystemStatus
}

func NewPlatformFilesystem(db *store.Store, storage *StorageManager, box *secretbox.Box, interval time.Duration, retention int) *PlatformFilesystem {
	return &PlatformFilesystem{db: db, storage: storage, box: box, interval: interval, retention: retention, status: PlatformFilesystemStatus{Prefix: platformFSPrefix, Encryption: "AES-256-GCM", BackupIntervalSec: int64(interval / time.Second), Retention: retention}}
}

func (f *PlatformFilesystem) Status() PlatformFilesystemStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()
	status := f.status
	if source, _, ok := f.storage.Primary(); ok {
		if status.SourceID != "" && status.SourceID != source.ID {
			status.Initialized = false
			status.LastSuccessAt = nil
		}
		status.SourceID, status.SourceName, status.SourceKind = source.ID, source.Name, source.Kind
	}
	return status
}

func (f *PlatformFilesystem) Run(ctx context.Context) {
	delay := 10 * time.Second
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		err := f.backup(ctx)
		if err != nil {
			delay = time.Minute
			if f.interval < delay {
				delay = f.interval
			}
		} else {
			delay = f.interval
		}
		next := time.Now().UTC().Add(delay)
		f.mu.Lock()
		f.status.NextAttemptAt = &next
		f.mu.Unlock()
	}
}

func (f *PlatformFilesystem) backup(ctx context.Context) error {
	now := time.Now().UTC()
	f.mu.Lock()
	f.status.LastAttemptAt = &now
	f.status.LastErrorCode = ""
	f.mu.Unlock()
	source, backend, ok := f.storage.Primary()
	if !ok {
		return f.fail("storage_unavailable", errors.New("no primary storage source"))
	}
	f.db.Audit(ctx, "platform-filesystem.backup-started", source.ID, "encrypted sqlite snapshot")
	snapshot, err := f.db.Snapshot(ctx)
	if err != nil {
		return f.fail("snapshot_failed", err)
	}
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err = zw.Write(snapshot); err != nil {
		return f.fail("compression_failed", err)
	}
	if err = zw.Close(); err != nil {
		return f.fail("compression_failed", err)
	}
	ciphertext, err := f.box.Seal(compressed.String())
	if err != nil {
		return f.fail("encryption_failed", err)
	}
	envelope, _ := json.Marshal(map[string]any{"format": "virsree-sqlite-backup-v1", "created_at": now, "compression": "gzip", "encryption": "AES-256-GCM", "payload": ciphertext})
	if !source.CapacityUnlimited && source.UsedBytes+source.ReservedBytes+int64(len(envelope)) > source.CapacityBytes {
		return f.fail("capacity_insufficient", errors.New("primary storage source has no room for platform snapshot"))
	}
	slot := int(now.Unix()/max(60, int64(f.interval/time.Second))) % f.retention
	backupKey := fmt.Sprintf("%s/backups/slot-%02d.rvsbak", platformFSPrefix, slot)
	if err = backend.Put(ctx, backupKey, bytes.NewReader(envelope), int64(len(envelope)), "application/vnd.virsree.backup+json"); err != nil {
		return f.fail("backup_write_failed", err)
	}
	marker, _ := json.MarshalIndent(map[string]any{"format": "virsree-filesystem-v1", "managed_by": "VirSree", "backup_prefix": platformFSPrefix + "/backups/", "retention": f.retention, "encryption": "AES-256-GCM", "updated_at": now}, "", "  ")
	if err = backend.Put(ctx, platformFSPrefix+"/filesystem.json", bytes.NewReader(marker), int64(len(marker)), "application/json"); err != nil {
		return f.fail("marker_write_failed", err)
	}
	f.db.Audit(ctx, "platform-filesystem.backup-succeeded", source.ID, fmt.Sprintf("slot=%02d encrypted_bytes=%d", slot, len(envelope)))
	f.mu.Lock()
	f.status.Initialized, f.status.SourceID, f.status.SourceName, f.status.SourceKind = true, source.ID, source.Name, source.Kind
	f.status.LastSuccessAt, f.status.LastErrorCode = &now, ""
	f.mu.Unlock()
	slog.Info("platform filesystem backup completed", "source_id", source.ID, "slot", slot, "encrypted_bytes", len(envelope))
	return nil
}

func (f *PlatformFilesystem) fail(code string, err error) error {
	f.mu.Lock()
	f.status.LastErrorCode = code
	f.mu.Unlock()
	slog.Warn("platform filesystem backup deferred", "error_code", code)
	return err
}
