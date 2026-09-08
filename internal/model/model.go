package model

import "time"

type Bucket struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	Visibility    string    `json:"visibility"`
	QuotaBytes    int64     `json:"quota_bytes"`
	UsedBytes     int64     `json:"used_bytes"`
	ReservedBytes int64     `json:"reserved_bytes"`
	CreatedAt     time.Time `json:"created_at"`
}

type AccessKey struct {
	ID           string    `json:"id"`
	BucketID     string    `json:"bucket_id"`
	Name         string    `json:"name"`
	AK           string    `json:"access_key"`
	SecretCipher string    `json:"-"`
	Permissions  string    `json:"permissions"`
	Revoked      bool      `json:"revoked"`
	CreatedAt    time.Time `json:"created_at"`
	LastUsedAt   time.Time `json:"last_used_at,omitempty"`
}

type Object struct {
	ID          string    `json:"id"`
	BucketID    string    `json:"bucket_id"`
	LogicalKey  string    `json:"key"`
	PhysicalKey string    `json:"-"`
	ContentType string    `json:"content_type"`
	ETag        string    `json:"etag"`
	Status      string    `json:"status"`
	Size        int64     `json:"size"`
	Generation  int64     `json:"generation"`
	Public      bool      `json:"public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BootstrapToken struct {
	ID, Name, TokenHash string
	MaxQuota            int64
	ExpiresAt           time.Time
	UsedAt              *time.Time
}

type Audit struct {
	ID                      int64
	Action, Subject, Detail string
	CreatedAt               time.Time
}
