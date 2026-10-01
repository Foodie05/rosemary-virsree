package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"rosemary-virsree/internal/provider"
	"rosemary-virsree/internal/secretbox"
)

func (s *Service) bucketLock(id string) *sync.RWMutex {
	v, _ := s.bucketLocks.LoadOrStore(id, &sync.RWMutex{})
	return v.(*sync.RWMutex)
}

// Operations already in progress finish before deletion is recorded. Credentials
// obtained earlier cannot bypass the durable state check after acquiring the lock.
func (s *Service) BucketOperation(ctx context.Context, id string) (func(), error) {
	if err := s.DB.RequireActiveBucket(ctx, id); err != nil {
		return nil, err
	}
	m := s.bucketLock(id)
	m.RLock()
	if err := s.DB.RequireActiveBucket(ctx, id); err != nil {
		m.RUnlock()
		return nil, err
	}
	return m.RUnlock, nil
}

func (s *Service) DeleteBucket(ctx context.Context, slug, confirmation, token string) error {
	if confirmation != slug {
		return errors.New("bucket name confirmation does not match")
	}
	b, err := s.DB.GetBucket(ctx, slug)
	if err != nil {
		return err
	}
	// Persist the intent immediately, even if a prior WebDAV transfer is slow.
	// The cleanup worker waits for in-flight operations before sweeping files.
	if err = s.DB.BeginBucketDeletion(ctx, b.ID, secretbox.Hash(token)); err != nil {
		return err
	}
	s.DB.Audit(ctx, "bucket.deletion-requested", b.ID, "physical cleanup queued")
	return nil
}

// Deletion survives browser disconnection and server restarts. Never remove the
// manifest on a provider failure, and sweep again after direct PUT grants expire.
func (s *Service) RunBucketCleanup(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		s.cleanupBuckets(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) cleanupBuckets(ctx context.Context) {
	jobs, err := s.DB.PendingBucketDeletions(ctx)
	if err != nil {
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		m := s.bucketLock(job.BucketID)
		m.Lock()
		work, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err = s.cleanBucket(work, job.BucketID, job.CleanupAfter)
		cancel()
		m.Unlock()
		if err != nil {
			// Provider errors can contain credentials, URLs and keys; log stable codes only.
			_ = s.DB.BucketDeletionError(ctx, job.BucketID, "bucket_cleanup_failed")
			slog.Warn("bucket cleanup incomplete", "bucket_id", job.BucketID, "code", "bucket_cleanup_failed")
		} else if time.Now().Before(job.CleanupAfter) {
			_ = s.DB.BucketDeletionError(ctx, job.BucketID, "")
		}
	}
}
func (s *Service) cleanBucket(ctx context.Context, id string, after time.Time) error {
	physicalSources, err := s.DB.BucketPhysicalSources(ctx, id)
	if err != nil {
		return err
	}
	for _, sourceID := range physicalSources {
		if sourceID == "" {
			if s.Storage.legacy == nil || !s.Storage.legacy.Ready() {
				return errors.New("legacy storage source missing during bucket cleanup")
			}
		} else {
			s.Storage.mu.RLock()
			_, ok := s.Storage.sources[sourceID]
			s.Storage.mu.RUnlock()
			if !ok {
				return errors.New("storage source missing during bucket cleanup")
			}
		}
	}
	sources, err := s.Storage.List(ctx)
	if err != nil {
		return err
	}
	var backends []provider.Backend
	for _, source := range sources {
		// Include disabled sources: they may still contain this bucket's files.
		s.Storage.mu.RLock()
		runtime, ok := s.Storage.sources[source.ID]
		s.Storage.mu.RUnlock()
		if !ok {
			return errors.New("storage source missing during bucket cleanup")
		}
		backends = append(backends, runtime.backend)
	}
	if s.Storage.legacy != nil && s.Storage.legacy.Ready() {
		backends = append(backends, s.Storage.legacy)
	}
	// Missing all sources must not turn a nonempty bucket into false success.
	if len(backends) == 0 {
		metrics, e := s.DB.BucketMetrics(ctx, id)
		if e != nil {
			return e
		}
		if metrics["object_count"].(int64) > 0 {
			return errors.New("no storage source for bucket cleanup")
		}
	}
	for _, backend := range backends {
		for _, prefix := range []string{"rosemary/" + id + "/", "rosemary-staging/" + id + "/"} {
			if err = backend.PurgePrefix(ctx, prefix); err != nil {
				return err
			}
		}
	}
	if time.Now().Before(after) {
		return nil
	}
	if err = s.DB.FinishBucketDeletion(ctx, id); err != nil {
		return err
	}
	s.DB.Audit(ctx, "bucket.deleted", id, "objects, upload grants, links, credentials and quotas cleaned")
	slog.Info("bucket cleanup completed", "bucket_id", id)
	return nil
}

func (s *Service) PrepareBucketDeletion(ctx context.Context, slug string) (string, time.Time, error) {
	b, err := s.DB.GetBucket(ctx, slug)
	if err != nil {
		return "", time.Time{}, err
	}
	token := secretbox.Random("delete_", 24)
	ready := time.Now().UTC().Add(5 * time.Second)
	err = s.DB.PrepareBucketDeletion(ctx, b.ID, secretbox.Hash(token), ready, ready.Add(15*time.Minute))
	return token, ready, err
}
