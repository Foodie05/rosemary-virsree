package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
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
	bucket   string
	client   *awss3.Client
	upload   *awss3.PresignClient
	download *awss3.PresignClient
	ready    bool
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
	return &S3{bucket: c.Bucket, client: makeClient(c.Endpoint), upload: awss3.NewPresignClient(makeClient(c.PublicEndpoint)), download: awss3.NewPresignClient(makeClient(c.DownloadEndpoint)), ready: true}
}
func (s *S3) Ready() bool  { return s.ready }
func (s *S3) Kind() string { return "s3" }
func (s *S3) Probe(ctx context.Context) error {
	if !s.ready {
		return fmt.Errorf("S3 backend is not configured")
	}
	key := fmt.Sprintf("rosemary-system/probes/%d", time.Now().UnixNano())
	copyKey := key + "-copy"
	putURL, err := s.PresignPut(ctx, key, "text/plain", 5, 2*time.Minute)
	if err != nil {
		return fmt.Errorf("sign upload probe: %w", err)
	}
	putReq, _ := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader([]byte("probe")))
	putReq.Header.Set("Content-Type", "text/plain")
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		return fmt.Errorf("direct upload probe: %w", err)
	}
	io.Copy(io.Discard, io.LimitReader(putResp.Body, 4096))
	putResp.Body.Close()
	if putResp.StatusCode >= 300 {
		return fmt.Errorf("direct upload probe: %s", putResp.Status)
	}
	defer s.Delete(context.Background(), key)
	defer s.Delete(context.Background(), copyKey)
	if _, err = s.Head(ctx, key); err != nil {
		return fmt.Errorf("metadata probe: %w", err)
	}
	getURL, err := s.PresignGet(ctx, key, 2*time.Minute, "")
	if err != nil {
		return fmt.Errorf("sign download probe: %w", err)
	}
	getReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		return fmt.Errorf("direct/CDN download probe: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(getResp.Body, 16))
	getResp.Body.Close()
	if getResp.StatusCode >= 300 {
		return fmt.Errorf("direct/CDN download probe: %s", getResp.Status)
	}
	if readErr != nil || string(body) != "probe" {
		return fmt.Errorf("direct/CDN download probe returned unexpected content")
	}
	if err = s.Copy(ctx, key, copyKey); err != nil {
		return fmt.Errorf("copy probe: %w", err)
	}
	if _, err = s.Head(ctx, copyKey); err != nil {
		return fmt.Errorf("copied object probe: %w", err)
	}
	if err = s.Delete(ctx, copyKey); err != nil {
		return fmt.Errorf("delete probe: %w", err)
	}
	if err = s.Delete(ctx, key); err != nil {
		return fmt.Errorf("delete probe: %w", err)
	}
	if _, err = s.Head(ctx, key); err == nil {
		return fmt.Errorf("delete probe: object still exists")
	}
	return nil
}
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
	r, e := s.upload.PresignPutObject(ctx, in, func(o *awss3.PresignOptions) { o.Expires = ttl })
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
	r, e := s.download.PresignGetObject(ctx, in, func(o *awss3.PresignOptions) { o.Expires = ttl })
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
func (s *S3) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if !s.ready {
		return fmt.Errorf("S3 backend is not configured")
	}
	in := &awss3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: body, ContentLength: aws.Int64(size)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	_, err := s.client.PutObject(ctx, in)
	return err
}
func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, Head, error) {
	if !s.ready {
		return nil, Head{}, fmt.Errorf("S3 backend is not configured")
	}
	r, err := s.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, Head{}, err
	}
	return r.Body, Head{Size: aws.ToInt64(r.ContentLength), ETag: aws.ToString(r.ETag), ContentType: aws.ToString(r.ContentType)}, nil
}
