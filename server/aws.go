package server

import (
	"context"
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (s Server) UploadToS3(filePath, key string) (string, error) {
	// S3 Configurations
	accessKeyID := s.AWS.AccessKey
	secretAccessKey := s.AWS.SecretKey
	awsRegion := s.AWS.Region
	bucketName := s.AWS.BucketName

	creds := aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""))

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(awsRegion),
		config.WithCredentialsProvider(creds),
	)
	if err != nil {
		log.Printf("Error loading AWS config: %v", err)
		return "", err
	}

	client := s3.NewFromConfig(cfg)

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error opening file %q: %v", filePath, err)
		return "", fmt.Errorf("unable to open file %q, %v", filePath, err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Printf("Error closing file %q: %v", filePath, cerr)
		}
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("unable to get file info for %q, %v", filePath, err)
	}
	fileSize := fileInfo.Size()

	// Set up file content type dynamically
	ext := filepath.Ext(filePath)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/json"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Upload the file to S3
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucketName),
		Key:           aws.String(key),
		Body:          file,
		ContentLength: &fileSize,
		ContentType:   aws.String(contentType),
		Metadata: map[string]string{
			"Content-Type": contentType,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload %q to S3: %v", filePath, err)
	}

	// Generate the file's S3 URL
	var url string
	if awsRegion == "us-east-1" {
		url = fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, key)
	} else {
		url = fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, awsRegion, key)
	}

	// Remove the temporary file after successful upload
	if err := os.Remove(filePath); err != nil {
		log.Printf("Warning: unable to remove local file %q: %v", filePath, err)
	}

	log.Printf("Successfully uploaded %q to S3, URL: %s", filePath, url)
	return url, nil
}
