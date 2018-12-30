package aws

import (
	"bytes"
	"fmt"
	sdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"net/http"
	"os"
)

type UploadTarget struct {
	Sourcepath string
	Bucket     string
	Dest       string
	//acl            ACLHandle
	//gen            int64 // a negative value indicates latest
	//conds          *Conditions
	//encryptionKey  []byte // AES-256 key
	//userProject    string // for requester-pays buckets
	//readCompressed bool   // Accept-Encoding: gzip
}

func Upload(session *session.Session, targets []UploadTarget) error {

	for _, target := range targets {

		file, err := os.Open(filename)
		if err != nil {
			return errors.WithMessage(err, "Failed to open file "+filename)
		}
		defer file.Close()

		fileInfo, _ := file.Stat()
		var size= fileInfo.Size()
		buffer := make([]byte, size)
		file.Read(buffer)

		output, err := s3.New(session).PutObject(&s3.PutObjectInput{
			Bucket:               sdk.String(bucket111),
			Key:                  sdk.String("delme/yarn.lock"),
			ACL:                  sdk.String("private"),
			Body:                 bytes.NewReader(buffer),
			ContentLength:        sdk.Int64(size),
			ContentType:          sdk.String(http.DetectContentType(buffer)),
			ContentDisposition:   sdk.String("attachment"),
			ServerSideEncryption: sdk.String("AES256"),
		})
	}

	if err != nil {
		return fmt.Errorf("failed to upload to AWS: %v", err)
	}

	//TODO: print output based on verbosity
	_ = output


	/*if false {
		errCh := make(chan error, len(uploadTargets))
		group := &sync.WaitGroup{}
		group.Add(len(uploadTargets))
		for dest, upload := range uploadTargets {

			//bucket *storage.BucketHandle
			//obj := bucket.Object(dest)
			var obj *storage.ObjectHandle = nil

			logrus.WithField("dest", dest).Info("Queued for upload")
			go func(f UploadFunc, obj *storage.ObjectHandle, name string) {
				defer group.Done()
				if err := f(obj); err != nil {
					errCh <- err
				}
				logrus.WithField("dest", name).Info("Finished upload")
			}(upload, obj, dest)
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
	}*/

	return nil
}