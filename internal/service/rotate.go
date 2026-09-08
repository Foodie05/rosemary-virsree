package service

import (
	"context"
	"time"
)

// RotateObjectKey copies an object to a fresh opaque physical key and deletes
// the old one. Previously issued direct S3 URLs then fail even before expiry.
func (s *Service) RotateObjectKey(ctx context.Context, c Credential, key string) error {
	o, err := s.DB.GetObject(ctx, c.Bucket.ID, key)
	if err != nil {
		return err
	}
	generation := time.Now().UnixNano()
	next := physical(c.Bucket, key, generation)
	if err = s.S3.Copy(ctx, o.PhysicalKey, next); err != nil {
		return err
	}
	if err = s.S3.Delete(ctx, o.PhysicalKey); err != nil {
		return err
	}
	if err = s.DB.RotatePhysicalKey(ctx, o.ID, next, generation); err != nil {
		// Restore the old physical key if the metadata switch fails. This keeps
		// the logical mapping readable even when the database update fails.
		_ = s.S3.Copy(ctx, next, o.PhysicalKey)
		_ = s.S3.Delete(ctx, next)
		return err
	}
	s.DB.Audit(ctx, "object.links-invalidated", c.Bucket.Slug, key)
	return nil
}
