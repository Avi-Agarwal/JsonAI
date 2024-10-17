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
		contentType = "application/json" // Default if type cannot be determined
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

func (s Server) UploadToS3Old(filePath, key string) (string, error) {
	accessKeyID := s.AWS.AccessKey
	secretAccessKey := s.AWS.SecretKey
	awsRegion := s.AWS.Region
	bucketName := s.AWS.BucketName

	creds := aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""))

	// Load the AWS default configuration
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(awsRegion), // Specify your AWS Region
		config.WithCredentialsProvider(creds),
	)
	if err != nil {
		log.Printf("Error in config.LoadDefaultConfig: %s", err)
		return "", err
	}

	// Create an S3 client
	client := s3.NewFromConfig(cfg)

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error in os.Open(): %s", err)
		return "", fmt.Errorf("unable to open file %q, %v", filePath, err)
	}
	defer func(file *os.File) {
		err = file.Close()
		if err != nil {
			log.Printf("Error in file.Close(): %s", err)
		}
	}(file)

	// Get the file info
	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("unable to get file info for %q, %v", filePath, err)
	}
	fileSize := fileInfo.Size()

	// Create the PutObject request
	_, err = client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(bucketName), // Replace with your bucket name
		Key:    aws.String(key),
		Body:   file,
		Metadata: map[string]string{
			"Content-Type": "image/png",
		},
		ContentLength: &fileSize,
		ContentType:   aws.String("image/png"),
	})

	if err != nil {
		return "", fmt.Errorf("unable to upload %q to %q, %v", filePath, "your-s3-bucket-name", err)
	}

	var url string
	if awsRegion == "us-east-1" {
		// The 'us-east-1' region does not require the region in the endpoint
		url = fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, key)
	} else {
		// For other regions, the region is part of the endpoint
		url = fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, awsRegion, key)
	}

	fmt.Println("URL of the generated image:", url)

	// Remove the file after uploading
	err = os.Remove(filePath)
	if err != nil {
		log.Printf("Error in os.Remove(): %s", err)
	}

	return url, nil
}
