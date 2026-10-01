package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// Never accept a physical bucket root, platform-filesystem prefix or partial ID.
func validPurgePrefix(prefix string) bool {
	p := strings.Split(prefix, "/")
	return len(p) == 3 && (p[0] == "rosemary" || p[0] == "rosemary-staging") && p[1] != "" && p[1] != "." && p[1] != ".." && p[2] == ""
}

func (s *S3) PurgePrefix(ctx context.Context, prefix string) error {
	if !s.ready || !validPurgePrefix(prefix) {
		return errors.New("invalid bucket cleanup prefix")
	}
	// Delete versions and delete markers too. Ordinary DELETE alone can retain all
	// bytes in a versioned private source. Only explicitly unsupported compatible
	// APIs fall back to a non-versioned sweep; permission errors remain failures.
	for {
		page, err := s.client.ListObjectVersions(ctx, &awss3.ListObjectVersionsInput{Bucket: aws.String(s.bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(1000)})
		if err != nil {
			var api smithy.APIError
			if errors.As(err, &api) && (api.ErrorCode() == "NotImplemented" || api.ErrorCode() == "MethodNotAllowed" || api.ErrorCode() == "NotSupported") {
				break
			}
			return err
		}
		for _, v := range page.Versions {
			if !strings.HasPrefix(aws.ToString(v.Key), prefix) {
				return errors.New("provider returned object outside cleanup prefix")
			}
			if _, err = s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: v.Key, VersionId: v.VersionId}); err != nil {
				return err
			}
		}
		for _, v := range page.DeleteMarkers {
			if !strings.HasPrefix(aws.ToString(v.Key), prefix) {
				return errors.New("provider returned marker outside cleanup prefix")
			}
			if _, err = s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: v.Key, VersionId: v.VersionId}); err != nil {
				return err
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		if len(page.Versions)+len(page.DeleteMarkers) == 0 {
			return errors.New("provider returned empty truncated cleanup page")
		}
	}
	for {
		page, err := s.client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{Bucket: aws.String(s.bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(1000)})
		if err != nil {
			return err
		}
		for _, v := range page.Contents {
			if !strings.HasPrefix(aws.ToString(v.Key), prefix) {
				return errors.New("provider returned object outside cleanup prefix")
			}
			if err = s.Delete(ctx, aws.ToString(v.Key)); err != nil {
				return err
			}
		}
		if len(page.Contents) == 0 && !aws.ToBool(page.IsTruncated) {
			return nil
		}
		if len(page.Contents) == 0 {
			return errors.New("provider returned empty truncated cleanup page")
		}
	}
}

func (w *WebDAV) PurgePrefix(ctx context.Context, prefix string) error {
	if !validPurgePrefix(prefix) {
		return errors.New("invalid bucket cleanup prefix")
	}
	// RFC 4918 collection DELETE includes descendants. A 207 is a partial failure.
	r, err := w.request(ctx, "DELETE", strings.TrimSuffix(prefix, "/"), nil, nil)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode == 200 || r.StatusCode == 204 || r.StatusCode == 404 {
		return nil
	}
	return fmt.Errorf("WebDAV collection cleanup: %s", r.Status)
}
