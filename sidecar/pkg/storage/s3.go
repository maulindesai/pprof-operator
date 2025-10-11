package storage

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-logr/logr"
	"github.com/maulindesai/pprof-operator/sidecar/pkg/metrics"
)

var logger logr.Logger

// SetLogger allows the sidecar main to inject a logger implementation
func SetLogger(l logr.Logger) {
	logger = l
}

// S3Config contains only the parameters needed to upload to S3/S3-compatible storage
type S3Config struct {
	S3Bucket         string
	S3Region         string
	S3PathPrefix     string
	S3Endpoint       string
	S3ForcePathStyle bool
	AWSAccessKeyID   string
	AWSSecretKey     string
}

// UploadToS3 uploads the file at filePath to the configured S3 bucket under the given s3Key.
func UploadToS3(ctx context.Context, cfg S3Config, filePath, s3Key string) error {
	logger.V(1).Info("Starting S3 upload process", "filePath", filePath, "s3Key", s3Key)

	// Create AWS config
	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		logger.Error(err, "Failed to load AWS config", "region", cfg.S3Region)
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	logger.V(2).Info("AWS config loaded successfully", "region", cfg.S3Region)

	// Create S3 client
	var s3Client *s3.Client
	if cfg.S3Endpoint != "" {
		logger.V(1).Info("Using custom S3-compatible endpoint", "endpoint", cfg.S3Endpoint, "forcePathStyle", cfg.S3ForcePathStyle)
		s3Client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.UsePathStyle = cfg.S3ForcePathStyle
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
		})
	} else {
		s3Client = s3.NewFromConfig(awsCfg)
	}
	logger.V(2).Info("S3 client created")

	// Open the file
	logger.V(2).Info("Opening file for upload", "path", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		logger.Error(err, "Failed to open file", "path", filePath)
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		logger.V(2).Info("Closing file", "path", filePath)
		if closeErr := file.Close(); closeErr != nil {
			logger.Error(closeErr, "Error closing file", "path", filePath)
			if err == nil {
				err = closeErr
			}
		}
	}()

	// Upload the file
	logger.Info("Uploading file to S3", "bucket", cfg.S3Bucket, "key", s3Key)
	startTime := time.Now()
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(cfg.S3Bucket),
		Key:    aws.String(s3Key),
		Body:   file,
	})
	if err != nil {
		// Increment upload error metric
		metrics.ProfileUploadErrors.Inc()
		logger.Error(err, "Failed to upload file to S3",
			"bucket", cfg.S3Bucket,
			"key", s3Key,
			"duration", time.Since(startTime))
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}

	// Increment upload success metric
	metrics.ProfilesUploaded.Inc()
	logger.Info("Successfully uploaded file to S3",
		"bucket", cfg.S3Bucket,
		"key", s3Key,
		"duration", time.Since(startTime))
	return nil
}

func loadAWSConfig(ctx context.Context, cfg S3Config) (aws.Config, error) {
	if cfg.AWSAccessKeyID != "" && cfg.AWSSecretKey != "" {
		return sdkconfig.LoadDefaultConfig(ctx,
			sdkconfig.WithRegion(cfg.S3Region),
			sdkconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				cfg.AWSAccessKeyID,
				cfg.AWSSecretKey,
				"",
			)),
		)
	}
	return sdkconfig.LoadDefaultConfig(ctx, sdkconfig.WithRegion(cfg.S3Region))
}
