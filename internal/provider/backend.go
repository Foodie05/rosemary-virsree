package provider

import (
	"context"
	"io"
	"time"
)

type Backend interface {
	Kind() string
	Ready() bool
	Probe(context.Context) error
	PresignPut(context.Context, string, string, int64, time.Duration) (string, error)
	PresignGet(context.Context, string, time.Duration, string) (string, error)
	Head(context.Context, string) (Head, error)
	Delete(context.Context, string) error
	Copy(context.Context, string, string) error
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, Head, error)
}
