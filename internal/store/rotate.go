package store

import (
	"context"
	"time"
)

func (s *Store) RotatePhysicalKey(ctx context.Context, id, newKey string, generation int64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE objects SET physical_key=?,generation=?,updated_at=? WHERE id=?", newKey, generation, time.Now().UTC(), id)
	return err
}
