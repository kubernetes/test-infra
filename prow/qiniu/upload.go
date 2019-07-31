package qiniu

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/qiniu/api.v7/auth"
	"github.com/qiniu/api.v7/storage"
	"github.com/sirupsen/logrus"
)

type UploadFunc func(*Uploader) error

type Uploader struct {
	Bucket    string
	AccessKey string
	SecretKey string

	qn  *storage.ResumeUploader
	mac *auth.Credentials
}

func NewUploader(bucket, accessKey, secretKey string) (*Uploader, error) {
	region, err := storage.GetRegion(accessKey, bucket)
	if err != nil {
		return nil, err
	}

	cfg := &storage.Config{
		Region:        region,
		UseHTTPS:      false,
		UseCdnDomains: false,
	}

	return &Uploader{
		Bucket: bucket,
		qn:     storage.NewResumeUploader(cfg),
		mac:    auth.New(accessKey, secretKey),
	}, nil
}

func FileUpload(file string, key string) UploadFunc {
	return func(ob *Uploader) error {
		putPolicy := storage.PutPolicy{
			Scope: ob.Bucket + ":" + key,
		}
		upToken := putPolicy.UploadToken(ob.mac)
		err := ob.qn.PutFile(context.Background(), &storage.PutRet{}, upToken, key, file, &storage.RputExtra{})
		return err
	}

}

// DataUpload returns an UploadFunc which copies all
// data from src reader into GCS
func DataUpload(key string, src io.Reader) UploadFunc {
	return func(ob *Uploader) error {
		putPolicy := storage.PutPolicy{
			Scope: ob.Bucket + ":" + key,
		}
		upToken := putPolicy.UploadToken(ob.mac)
		err := ob.qn.PutWithoutSize(context.Background(), &storage.PutRet{}, upToken, key, src, &storage.RputExtra{})
		return err
	}
}

// Upload uploads all of the data in the
// uploadTargets map to GCS in parallel. The map is
// keyed on GCS path under the bucket
func (up *Uploader) Upload(uploadTargets map[string]UploadFunc) error {
	errCh := make(chan error, len(uploadTargets))
	group := &sync.WaitGroup{}
	group.Add(len(uploadTargets))
	for dest, upload := range uploadTargets {
		logrus.WithField("dest", dest).Info("Queued for upload")
		go func(f UploadFunc, obj *Uploader, name string) {
			defer group.Done()
			if err := f(obj); err != nil {
				errCh <- err
			}
			logrus.WithField("dest", name).Info("Finished upload")
		}(upload, up, dest)
	}
	group.Wait()
	close(errCh)
	if len(errCh) != 0 {
		var uploadErrors []error
		for err := range errCh {
			uploadErrors = append(uploadErrors, err)
		}
		return fmt.Errorf("encountered errors during upload: %v", uploadErrors)
	}

	logrus.Info("Finished upload to GCS")

	return nil
}
