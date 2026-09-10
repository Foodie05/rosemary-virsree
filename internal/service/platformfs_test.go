package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"
	"time"

	"rosemary-virsree/internal/model"
	"rosemary-virsree/internal/provider"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/store"
)

type memoryBackend struct{ objects map[string][]byte }

func (m *memoryBackend) Kind() string                { return "s3" }
func (m *memoryBackend) Ready() bool                 { return true }
func (m *memoryBackend) Probe(context.Context) error { return nil }
func (m *memoryBackend) PresignPut(context.Context, string, string, int64, time.Duration) (string, error) {
	return "", nil
}
func (m *memoryBackend) PresignGet(context.Context, string, time.Duration, string) (string, error) {
	return "", nil
}
func (m *memoryBackend) Head(_ context.Context, k string) (provider.Head, error) {
	return provider.Head{Size: int64(len(m.objects[k]))}, nil
}
func (m *memoryBackend) Delete(_ context.Context, k string) error { delete(m.objects, k); return nil }
func (m *memoryBackend) Copy(_ context.Context, a, b string) error {
	m.objects[b] = append([]byte(nil), m.objects[a]...)
	return nil
}
func (m *memoryBackend) Put(_ context.Context, k string, r io.Reader, _ int64, _ string) error {
	m.objects[k], _ = io.ReadAll(r)
	return nil
}
func (m *memoryBackend) Get(_ context.Context, k string) (io.ReadCloser, provider.Head, error) {
	v := m.objects[k]
	return io.NopCloser(bytes.NewReader(v)), provider.Head{Size: int64(len(v))}, nil
}

func TestPlatformFilesystemWritesDecryptableRollingSnapshot(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "virsree.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	box, _ := secretbox.New("backup-test-master-key")
	backend := &memoryBackend{objects: map[string][]byte{}}
	manager := NewStorageManager(db, box, "https://example.test", nil)
	source := model.StorageSource{ID: "primary", Name: "Primary", Kind: "s3", Priority: 1, CapacityUnlimited: true, Enabled: true, CreatedAt: time.Now().UTC()}
	manager.sources[source.ID] = sourceRuntime{meta: source, backend: backend}
	fs := NewPlatformFilesystem(db, manager, box, time.Hour, 3)
	if err = fs.backup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := backend.objects[platformFSPrefix+"/filesystem.json"]; !ok {
		t.Fatal("filesystem marker missing")
	}
	var backup []byte
	for key, value := range backend.objects {
		if len(key) > 7 && key[len(key)-7:] == ".rvsbak" {
			backup = value
		}
	}
	if len(backup) == 0 {
		t.Fatal("encrypted snapshot missing")
	}
	var envelope struct {
		Payload string `json:"payload"`
	}
	if err = json.Unmarshal(backup, &envelope); err != nil {
		t.Fatal(err)
	}
	compressed, err := box.Open(envelope.Payload)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewBufferString(compressed))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) < 16 || string(plain[:15]) != "SQLite format 3" {
		t.Fatal("decrypted payload is not a SQLite database")
	}
	if !fs.Status().Initialized {
		t.Fatal("filesystem status was not updated")
	}
}
