package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

const cleanupHours = 12
const startedadDateFormat = "2006-01-02T15:04:05Z"

func main() {

	endPoint, bucket, accessKey, secretAccessKey := getCommandLineArgs()

	totalRemoved := 0
	s := getS3Client(endPoint, accessKey, secretAccessKey)

	fmt.Printf("Endpoint: %s\n", *s.Config.Endpoint)
	fmt.Printf("Bucket: %s\n\n", bucket)

	objs, err := s.ListObjects(&s3.ListObjectsInput{
		Bucket:    aws.String(bucket),
		Prefix:    aws.String("docker/registry/v2/repositories/"),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		panic(err)
	}

	for i, cp := range objs.CommonPrefixes {
		fmt.Printf("Prefix %d: %s\n", i, *cp.Prefix)

		resp, err := s.ListMultipartUploads(&s3.ListMultipartUploadsInput{
			Bucket:     aws.String(bucket),
			Prefix:     aws.String(*cp.Prefix),
			MaxUploads: aws.Int64(1000),
		})
		if err != nil {
			panic(err)
		}

		fmt.Printf(" # of uploads found for prefix: %d\n", len(resp.Uploads))

		for i, multi := range resp.Uploads {
			fmt.Printf("  Upload %d: %s\n", i, *multi.Key)

			hoursSince, err := hoursSinceUploadStarted(s, bucket, *multi.Key)
			if err != nil {
				fmt.Printf(" ERROR: %s\n", err)
				continue
			}

			fmt.Printf("  Started %d hours ago\n", hoursSince)

			if hoursSince > cleanupHours {
				_, err = s.AbortMultipartUpload(&s3.AbortMultipartUploadInput{
					Bucket:   aws.String(bucket),
					Key:      multi.Key,
					UploadId: multi.UploadId,
				})
				if err != nil {
					fmt.Printf(" ERROR: %s", err)
				} else {
					totalRemoved++
					fmt.Printf("  Removed, for a total of %d uploads\n", totalRemoved)
				}

			}

		}
	}

}

func getCommandLineArgs() (string, string, string, string) {
	if len(os.Args) != 5 {
		fmt.Printf("Usage: %s <endpoint> <bucketname> <accessKey> <secretKey>", os.Args[0])
		os.Exit(1)
	}
	return os.Args[1], os.Args[2], os.Args[3], os.Args[4]
}

func getS3Client(endPoint, accessKey, secretAccessKey string) *s3.S3 {
	awsConfig := aws.NewConfig()

	creds := credentials.NewChainCredentials([]credentials.Provider{
		&credentials.StaticProvider{
			Value: credentials.Value{
				AccessKeyID:     accessKey,
				SecretAccessKey: secretAccessKey,
			},
		},
		&credentials.EnvProvider{},
		&credentials.SharedCredentialsProvider{},
		&ec2rolecreds.EC2RoleProvider{Client: ec2metadata.New(session.New())},
	})

	awsConfig.WithS3ForcePathStyle(true)
	awsConfig.WithEndpoint(endPoint)

	awsConfig.WithCredentials(creds)
	awsConfig.WithRegion("us-west-1")
	awsConfig.WithDisableSSL(true)

	return s3.New(session.New(awsConfig))
}

func hoursSinceUploadStarted(s *s3.S3, bucket, key string) (int, error) {
	startedat := strings.TrimRight(key, "data") + "startedat"
	obj, err := s.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(startedat),
	})

	if err != nil {
		return 0, err
	}

	defer obj.Body.Close()
	t, err := parseTimeFromStream(obj.Body)
	if err != nil {
		panic(err)
	}

	return int(time.Since(t).Hours()), nil
}

func parseTimeFromStream(s io.Reader) (time.Time, error) {
	buf := new(bytes.Buffer)

	_, err := buf.ReadFrom(s)
	if err != nil {
		return time.Time{}, err
	}

	dateString := buf.String()
	return time.Parse(startedadDateFormat, dateString)
}
