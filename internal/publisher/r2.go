package publisher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type R2Options struct {
	AccountID       string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
}

type S3Store struct {
	Bucket string
	Client s3PutObjectClient
}

type s3PutObjectClient interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func NewR2Store(ctx context.Context, options R2Options) (S3Store, error) {
	resolved, err := resolveR2Options(options)
	if err != nil {
		return S3Store{}, err
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(resolved.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(resolved.AccessKeyID, resolved.SecretAccessKey, "")),
	)
	if err != nil {
		return S3Store{}, err
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(resolved.Endpoint)
		options.UsePathStyle = true
	})
	return S3Store{Bucket: resolved.Bucket, Client: client}, nil
}

func (s S3Store) PutObject(ctx context.Context, object Object) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bucket := strings.TrimSpace(s.Bucket)
	if bucket == "" {
		return errors.New("S3 bucket is required")
	}
	if s.Client == nil {
		return errors.New("S3 client is required")
	}
	if strings.TrimSpace(object.Key) == "" {
		return errors.New("object key is required")
	}
	body, size, closeBody, err := objectBody(object)
	if err != nil {
		return err
	}
	if closeBody != nil {
		defer closeBody()
	}
	contentType := object.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	input := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(object.Key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
		Metadata:      s3Metadata(object.Metadata),
	}
	if !object.AllowOverwrite {
		input.IfNoneMatch = aws.String("*")
	}
	if _, err := s.Client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("put object %s: %w", object.Key, err)
	}
	return nil
}

type resolvedR2Options struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
}

func resolveR2Options(options R2Options) (resolvedR2Options, error) {
	endpoint := strings.TrimSpace(options.Endpoint)
	accountID := strings.TrimSpace(options.AccountID)
	if endpoint == "" {
		if accountID == "" {
			return resolvedR2Options{}, errors.New("R2 account id or endpoint is required")
		}
		endpoint = "https://" + accountID + ".r2.cloudflarestorage.com"
	}
	endpoint = strings.TrimRight(endpoint, "/")
	bucket := strings.TrimSpace(options.Bucket)
	if bucket == "" {
		return resolvedR2Options{}, errors.New("R2 bucket is required")
	}
	accessKeyID := strings.TrimSpace(options.AccessKeyID)
	if accessKeyID == "" {
		return resolvedR2Options{}, errors.New("R2 access key id is required")
	}
	secretAccessKey := strings.TrimSpace(options.SecretAccessKey)
	if secretAccessKey == "" {
		return resolvedR2Options{}, errors.New("R2 secret access key is required")
	}
	region := strings.TrimSpace(options.Region)
	if region == "" {
		region = "auto"
	}
	return resolvedR2Options{
		Endpoint:        endpoint,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Bucket:          bucket,
		Region:          region,
	}, nil
}

func objectBody(object Object) (io.Reader, int64, func(), error) {
	if object.SourcePath != "" {
		file, err := os.Open(object.SourcePath)
		if err != nil {
			return nil, 0, nil, err
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, 0, nil, err
		}
		return file, info.Size(), func() { _ = file.Close() }, nil
	}
	return bytes.NewReader(object.Body), int64(len(object.Body)), nil, nil
}

func s3Metadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	clean := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		clean[strings.ToLower(key)] = value
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}
