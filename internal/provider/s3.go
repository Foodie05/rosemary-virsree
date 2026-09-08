package provider

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"rosemary-virsree/internal/config"
)

const MaxSigV4TTL = 7 * 24 * time.Hour

type S3 struct {
	bucket string
	client *awss3.Client
	public *awss3.PresignClient
	ready  bool
}
type Head struct {
	Size              int64
	ETag, ContentType string
}

func New(c config.Backend) *S3 {
	if c.Bucket == "" || c.AccessKey == "" || c.SecretKey == "" {
		return &S3{}
	}
	base := aws.Config{Region: c.Region, Credentials: credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, "")}
	makeClient := func(endpoint string) *awss3.Client {
		return awss3.NewFromConfig(base, func(o *awss3.Options) {
			o.UsePathStyle = c.PathStyle
			if endpoint != "" {
				o.BaseEndpoint = aws.String(endpoint)
			}
		})
	}
	return &S3{bucket: c.Bucket, client: makeClient(c.Endpoint), public: awss3.NewPresignClient(makeClient(c.PublicEndpoint)), ready: true}
}
func (s *S3) Ready() bool { return s.ready }
func validTTL(ttl time.Duration) error {
	if ttl < time.Second {
		return fmt.Errorf("expires_in must be at least 1 second")
	}
	if ttl > MaxSigV4TTL {
		return fmt.Errorf("expires_in exceeds S3 SigV4 limit of 604800 seconds")
	}
	return nil
}
func (s *S3) PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (string, error) {
	if !s.ready {
		return "", fmt.Errorf("S3 backend is not configured")
	}
	if e := validTTL(ttl); e != nil {
		return "", e
	}
	in := &awss3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), ContentLength: aws.Int64(size)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	r, e := s.public.PresignPutObject(ctx, in, func(o *awss3.PresignOptions) { o.Expires = ttl })
	if e != nil {
		return "", e
	}
	return r.URL, nil
}
func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration, downloadName string) (string, error) {
	if !s.ready {
		return "", fmt.Errorf("S3 backend is not configured")
	}
	if e := validTTL(ttl); e != nil {
		return "", e
	}
	in := &awss3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	if downloadName != "" {
		in.ResponseContentDisposition = aws.String(`attachment; filename="` + strings.ReplaceAll(downloadName, `"`, "") + `"`)
	}
	r, e := s.public.PresignGetObject(ctx, in, func(o *awss3.PresignOptions) { o.Expires = ttl })
	if e != nil {
		return "", e
	}
	return r.URL, nil
}
func (s *S3) Head(ctx context.Context, key string) (Head, error) {
	if !s.ready {
		return Head{}, fmt.Errorf("S3 backend is not configured")
	}
	r, e := s.client.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if e != nil {
		return Head{}, e
	}
	return Head{Size: aws.ToInt64(r.ContentLength), ETag: aws.ToString(r.ETag), ContentType: aws.ToString(r.ContentType)}, nil
}
func (s *S3) Delete(ctx context.Context, key string) error {
	if !s.ready {
		return fmt.Errorf("S3 backend is not configured")
	}
	_, e := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	return e
}
func (s *S3) Copy(ctx context.Context, from, to string) error {
	if !s.ready {
		return fmt.Errorf("S3 backend is not configured")
	}
	src := url.PathEscape(s.bucket + "/" + from)
	_, e := s.client.CopyObject(ctx, &awss3.CopyObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(to), CopySource: aws.String(src)})
	return e
}
