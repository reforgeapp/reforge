package artifact

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/reforgeapp/reforge/pkg/store"
)

type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

type s3Blobs struct {
	client *s3.Client
	bucket string
}

func NewS3(ctx context.Context, db *store.Store, cfg S3Config) (*Store, error) {
	if cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, errors.New("S3 artifact storage requires a bucket and credentials")
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	client := s3.New(s3.Options{
		Region:                     region,
		Credentials:                credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		UsePathStyle:               cfg.Endpoint != "",
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})
	check, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := client.HeadBucket(check, &s3.HeadBucketInput{Bucket: aws.String(cfg.Bucket)}); err != nil {
		return nil, errors.New("S3 artifact bucket is not reachable: " + err.Error())
	}
	return &Store{db: db, blobs: s3Blobs{client: client, bucket: cfg.Bucket}}, nil
}

func (b s3Blobs) close() error { return nil }

func (b s3Blobs) put(ctx context.Context, name string, data []byte) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(b.bucket), Key: aws.String(name), Body: bytes.NewReader(data), ContentLength: aws.Int64(int64(len(data)))})
	return err
}

func (b s3Blobs) get(ctx context.Context, name string) ([]byte, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(b.bucket), Key: aws.String(name)})
	var missing *types.NoSuchKey
	if errors.As(err, &missing) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(io.LimitReader(out.Body, MaxSize+1))
}

func (b s3Blobs) remove(ctx context.Context, name string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(b.bucket), Key: aws.String(name)})
	return err
}

func (b s3Blobs) list(ctx context.Context, fn func(string, time.Time) error) error {
	pages := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{Bucket: aws.String(b.bucket)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, object := range page.Contents {
			if err = fn(aws.ToString(object.Key), aws.ToTime(object.LastModified)); err != nil {
				return err
			}
		}
	}
	return nil
}
