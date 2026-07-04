package storage

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"archivesync/internal/models"
)

func init() {
	Register(models.ChannelS3, newS3)
}

// s3Backend is a storage.Backend backed by S3 or an S3-compatible service such
// as Cloudflare R2 or MinIO.
type s3Backend struct {
	client *s3.Client
	bucket string
	prefix string // key prefix within the bucket (no surrounding slashes)
}

// newS3 constructs an S3 / S3-compatible backend from a channel definition.
// It requires ch.Config.Bucket. A custom Endpoint (R2/MinIO), Region and
// ForcePathStyle are honoured when set.
func newS3(ch models.Channel) (Backend, error) {
	bucket := strings.TrimSpace(ch.Config.Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("storage/s3: bucket is required")
	}

	region := ch.Config.Region
	if region == "" {
		region = "us-east-1"
	}

	client := s3.NewFromConfig(aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(ch.Config.AccessKeyID, ch.Config.SecretAccessKey, ""),
	}, func(o *s3.Options) {
		if ch.Config.Endpoint != "" {
			o.BaseEndpoint = aws.String(ch.Config.Endpoint)
		}
		o.UsePathStyle = ch.Config.ForcePathStyle
	})

	return &s3Backend{
		client: client,
		bucket: bucket,
		prefix: strings.Trim(ch.Config.Prefix, "/"),
	}, nil
}

// fullKey joins the configured prefix with key, yielding a forward-slash key
// with no leading slash.
func (b *s3Backend) fullKey(key string) string {
	return strings.TrimPrefix(path.Join(b.prefix, key), "/")
}

// fullPrefix is like fullKey but preserves a trailing slash in the caller's
// prefix. This is essential for List: retention lists "<dir>/" to establish a
// per-target directory boundary, and a byte-prefix like "app" (slash stripped)
// would over-match sibling directories such as "app-db/" and cause cross-target
// deletion. path.Join/Clean would strip the slash, so we re-append it.
func (b *s3Backend) fullPrefix(prefix string) string {
	fp := b.fullKey(prefix)
	if fp != "" && strings.HasSuffix(prefix, "/") && !strings.HasSuffix(fp, "/") {
		fp += "/"
	}
	return fp
}

// stripKey removes the configured prefix from a returned object key so callers
// observe the same key space they used with Put.
func (b *s3Backend) stripKey(key string) string {
	if b.prefix == "" {
		return key
	}
	trimmed := strings.TrimPrefix(key, b.prefix)
	return strings.TrimPrefix(trimmed, "/")
}

// Put streams r to the object at key. size may be -1; the uploader handles an
// unknown length.
func (b *s3Backend) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	fk := b.fullKey(key)
	uploader := manager.NewUploader(b.client)
	if _, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: &b.bucket,
		Key:    &fk,
		Body:   r,
	}); err != nil {
		return fmt.Errorf("storage/s3: put %q: %w", key, err)
	}
	return nil
}

// Get opens the object at key for reading.
func (b *s3Backend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fk := b.fullKey(key)
	resp, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &b.bucket,
		Key:    &fk,
	})
	if err != nil {
		return nil, fmt.Errorf("storage/s3: get %q: %w", key, err)
	}
	return resp.Body, nil
}

// List returns objects whose key starts with prefix.
func (b *s3Backend) List(ctx context.Context, prefix string) ([]Object, error) {
	fp := b.fullPrefix(prefix)
	input := &s3.ListObjectsV2Input{
		Bucket: &b.bucket,
	}
	if fp != "" {
		input.Prefix = &fp
	}

	var out []Object
	paginator := s3.NewListObjectsV2Paginator(b.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("storage/s3: list %q: %w", prefix, err)
		}
		for _, obj := range page.Contents {
			key := b.stripKey(aws.ToString(obj.Key))
			// Defensive: ensure the logical key really is under the requested
			// prefix boundary (guards against any residual over-match).
			if prefix != "" && !strings.HasPrefix(key, prefix) {
				continue
			}
			out = append(out, Object{
				Key:          key,
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
			})
		}
	}
	return out, nil
}

// Delete removes the object at key.
func (b *s3Backend) Delete(ctx context.Context, key string) error {
	fk := b.fullKey(key)
	if _, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &b.bucket,
		Key:    &fk,
	}); err != nil {
		return fmt.Errorf("storage/s3: delete %q: %w", key, err)
	}
	return nil
}

// Ping verifies bucket access and credentials.
func (b *s3Backend) Ping(ctx context.Context) error {
	if _, err := b.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &b.bucket,
	}); err != nil {
		return fmt.Errorf("storage/s3: ping bucket %q: %w", b.bucket, err)
	}
	return nil
}

// Kind returns the backend type identifier.
func (b *s3Backend) Kind() string { return "s3" }
