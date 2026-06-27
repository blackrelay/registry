package publisher

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestResolveR2OptionsBuildsCloudflareEndpoint(t *testing.T) {
	resolved, err := resolveR2Options(R2Options{
		AccountID:       "account-123",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
		Bucket:          "registry-exports",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Endpoint != "https://account-123.r2.cloudflarestorage.com" {
		t.Fatalf("unexpected endpoint %q", resolved.Endpoint)
	}
	if resolved.Region != "auto" {
		t.Fatalf("unexpected region %q", resolved.Region)
	}
}

func TestResolveR2OptionsRequiresCredentialsWithoutEchoingSecret(t *testing.T) {
	_, err := resolveR2Options(R2Options{
		AccountID:       "",
		AccessKeyID:     "key",
		SecretAccessKey: "secret-value",
		Bucket:          "registry-exports",
	})
	if err == nil {
		t.Fatal("expected missing account id or endpoint to fail")
	}
	if bytes.Contains([]byte(err.Error()), []byte("secret-value")) {
		t.Fatalf("error leaked secret: %v", err)
	}
}

func TestS3StoreUsesConditionalWritesForImmutableObjects(t *testing.T) {
	client := &fakeS3Client{}
	store := S3Store{Bucket: "registry-exports", Client: client}
	err := store.PutObject(context.Background(), Object{
		Key:         "registry/bundles/bundle/catalog.json",
		Body:        []byte("{}\n"),
		ContentType: "application/json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.inputs) != 1 {
		t.Fatalf("expected one put request, got %d", len(client.inputs))
	}
	input := client.inputs[0]
	if input.IfNoneMatch == nil || *input.IfNoneMatch != "*" {
		t.Fatalf("immutable object was not conditionally written: %#v", input.IfNoneMatch)
	}
	if input.ContentLength == nil || *input.ContentLength != 3 {
		t.Fatalf("unexpected content length: %#v", input.ContentLength)
	}
}

func TestS3StoreAllowsLatestPointerOverwrite(t *testing.T) {
	client := &fakeS3Client{}
	store := S3Store{Bucket: "registry-exports", Client: client}
	err := store.PutObject(context.Background(), Object{
		Key:            "registry/latest/manifest.json",
		Body:           []byte("{}\n"),
		ContentType:    "application/json",
		AllowOverwrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	input := client.inputs[0]
	if input.IfNoneMatch != nil {
		t.Fatalf("latest pointer should allow overwrite, got condition %#v", input.IfNoneMatch)
	}
}

type fakeS3Client struct {
	inputs []*s3.PutObjectInput
}

func (c *fakeS3Client) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if input.Body != nil {
		_, _ = io.ReadAll(input.Body)
	}
	c.inputs = append(c.inputs, input)
	return &s3.PutObjectOutput{}, nil
}
