package awsapi

import (
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

func S3Download(handle *BucketHandle, key string) {
	downloader := s3manager.NewDownloader(handle.client.session)
	_ = downloader

	//_, err := downloader.Download(&s3manager.UploadInput{
		//aws.String(handle.bucket),
		//aws.String(key),

}