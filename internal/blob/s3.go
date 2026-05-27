package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type S3Config struct {
	Endpoint     string
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool
}

type S3Store struct {
	client *s3.Client
	bucket string
}

func NewS3(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("missing s3 configuration")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.UsePathStyle
	})
	return &S3Store{client: client, bucket: cfg.Bucket}, nil
}

func s3Key(bucket, userKey, fileKey string) string {
	return path.Join(bucket, userKey, fileKey)
}

func (s *S3Store) Put(ctx context.Context, bucket, userKey, fileKey string, data []byte) (string, error) {
	key := s3Key(bucket, userKey, fileKey)
	contentLength := int64(len(data))
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(contentLength),
	})
	if err != nil {
		return "", fmt.Errorf("put object: %w", err)
	}
	return key, nil
}

func (s *S3Store) Get(ctx context.Context, relPath string, w io.Writer) error {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(relPath),
	})
	if err != nil {
		return mapS3Error(err)
	}
	defer out.Body.Close()
	_, err = io.Copy(w, out.Body)
	return err
}

func (s *S3Store) Delete(ctx context.Context, relPath string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(relPath),
	})
	if err != nil {
		return mapS3Error(err)
	}
	return nil
}

func (s *S3Store) Stat(ctx context.Context, relPath string) (int64, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(relPath),
	})
	if err != nil {
		return 0, mapS3Error(err)
	}
	return aws.ToInt64(out.ContentLength), nil
}

func mapS3Error(err error) error {
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return ErrNotFound
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return ErrNotFound
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return ErrNotFound
		}
	}
	return err
}
