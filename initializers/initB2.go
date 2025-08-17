package initializers

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var S3Client *s3.Client
var BucketName = os.Getenv("B2_BUCKET_NAME")

func InitB2() {
	keyID := os.Getenv("B2_KEY_ID")
	appKey := os.Getenv("B2_APP_KEY")
	region := os.Getenv("B2_REGION") // e.g. "us-west-002"

	customResolver := aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:           "https://s3." + region + ".backblazeb2.com",
			SigningRegion: region,
		}, nil
	})

	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(keyID, appKey, "")),
		config.WithEndpointResolver(customResolver),
	)
	if err != nil {
		log.Fatalf("Unable to load B2 config: %v", err)
	}

	S3Client = s3.NewFromConfig(cfg)
}
