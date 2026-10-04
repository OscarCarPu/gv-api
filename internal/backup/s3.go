package backup

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Uploader struct {
	client *s3.Client
	bucket string
}

func NewS3Uploader(ctx context.Context, bucket string) (*S3Uploader, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &S3Uploader{client: s3.NewFromConfig(cfg), bucket: bucket}, nil
}

func (u *S3Uploader) Upload(ctx context.Context, key, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(u.bucket),
		Key:         aws.String("gv-db/" + key),
		Body:        f,
		IfNoneMatch: aws.String("*"),
	})
	return err
}
