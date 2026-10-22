package failover

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type SSM struct {
	client *ssm.Client
}

func NewSSM(ctx context.Context) (*SSM, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &SSM{client: ssm.NewFromConfig(cfg)}, nil
}

func (s *SSM) Get(ctx context.Context, name string) (string, error) {
	out, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(name)})
	var missing *ssmtypes.ParameterNotFound
	if errors.As(err, &missing) {
		return "", ErrNoParam
	}
	if err != nil {
		return "", err
	}
	return aws.ToString(out.Parameter.Value), nil
}

type S3 struct {
	client *s3.Client
	bucket string
}

func NewS3(ctx context.Context, bucket string) (*S3, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		// A custom endpoint (the local fake AWS) has no bucket.host name to resolve.
		o.UsePathStyle = os.Getenv("AWS_ENDPOINT_URL") != ""
	})
	return &S3{client: client, bucket: bucket}, nil
}

// Download writes the object to dest through a temporary file, so dest is never half written.
func (s *S3) Download(ctx context.Context, key, dest string) error {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return err
	}
	defer out.Body.Close()

	tmp := dest + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if _, err := io.Copy(f, out.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
