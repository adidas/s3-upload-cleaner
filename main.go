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

	if *objs.IsTruncated {
		panic("Output is truncated. TO-DO: implement pagination")
	}

	for i, cp := range objs.CommonPrefixes {
		fmt.Printf("Prefix %d: %s\n", i, *cp.Prefix)

		totalRemoved += cleanMPUs(s, bucket, *cp.Prefix)
		fmt.Printf("  Total MPUs removed: %d\n", totalRemoved)
	}

	fmt.Println()
	fmt.Println("Removing upload folders:")
	cleanUploadFolders(s, bucket, *objs.Prefix)

}

func cleanMPUs(s *s3.S3, bucket, prefix string) (totalRemoved int) {
	totalRemoved = 0

	resp, err := s.ListMultipartUploads(&s3.ListMultipartUploadsInput{
		Bucket:     aws.String(bucket),
		Prefix:     aws.String(prefix),
		MaxUploads: aws.Int64(1000),
	})

	if err != nil {
		panic(err)
	}

	if *resp.IsTruncated {
		panic("Output is truncated. TO-DO: implement pagination")
	}

	fmt.Printf(" # of MPUs found for prefix: %d\n", len(resp.Uploads))

	for i, multi := range resp.Uploads {
		fmt.Printf("  Upload %d: %s\n", i, *multi.Key)

		hoursSince := int(time.Since(*multi.Initiated).Hours())

		fmt.Printf("  Started %d hours ago\n", hoursSince)

		if hoursSince > cleanupHours {
			_, err = s.AbortMultipartUpload(&s3.AbortMultipartUploadInput{
				Bucket:   aws.String(bucket),
				Key:      multi.Key,
				UploadId: multi.UploadId,
			})

			if err != nil {
				fmt.Printf(" ERROR: %s\n", err)
			} else {
				fmt.Println("   Removed!")
				totalRemoved++
			}
		}
	}

	return
}

func cleanUploadFolders(s *s3.S3, bucket, prefix string) {
	shouldContinue := true
	var continuationToken *string
	for shouldContinue {

		objs, err := s.ListObjectsV2(&s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(prefix),
			MaxKeys:           aws.Int64(100),
			ContinuationToken: continuationToken,
		})

		if err != nil {
			panic(err)
		}

		for _, o := range objs.Contents {
			if strings.Contains(*o.Key, "/_uploads/") && strings.HasSuffix(*o.Key, "/startedat") {
				hoursSince, err := hoursSinceUploadStarted(s, bucket, *o.Key)
				if err != nil {
					fmt.Printf(" ERROR: %s\n", err)
					continue
				}

				if hoursSince > cleanupHours {
					fmt.Printf("  Removing folder %s (%d hours)\n", *o.Key, hoursSince)
					removeUploadFolder(s, bucket, *o.Key)
				} else {
					fmt.Printf("  Skipping folder %s (%d hours)\n", *o.Key, hoursSince)
				}
			}
		}

		continuationToken = objs.NextContinuationToken
		shouldContinue = *objs.IsTruncated
	}
}

func removeUploadFolder(s *s3.S3, bucket, prefix string) {
	keyParts := strings.Split(prefix, "/")
	uploadsFolder := strings.Join(keyParts[0:len(keyParts)-1], "/")

	objs, err := s.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(uploadsFolder),
	})

	if err != nil {
		panic(err)
	}

	for _, o := range objs.Contents {
		_, err := s.DeleteObject(&s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    o.Key,
		})

		if err != nil {
			panic(err)
		}

		fmt.Printf("    Removing %s\n", *o.Key)
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
	obj, err := s.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
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
