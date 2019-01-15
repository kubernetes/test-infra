package awsapi

import (
	"io"
	"k8s.io/test-infra/bazel-test-infra/external/go_sdk/src/context"
	"os"
	"testing"
	"time"
)

func Test_S3Upload(t *testing.T) {
	opt := ClientOption {
			CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(opt)
	b := client.Bucket("dev-okro-io")
	o := b.Object("lala")
	w := o.NewWriter(context.Background())

	file, err := os.Open("/Users/Traiana/alexa/Downloads/f1.txt")

	if err != nil {
		t.Error(err)
	}

	defer file.Close()

	io.Copy(w, &SlowReader{file})
	w.Close()
}

// simulate slow read, to let S3 upload wait a while
type SlowReader struct {
	r *os.File
}

func (s *SlowReader) Read(buffer []byte) (n int, err error) {
	time.Sleep(5 * time.Millisecond)
	return s.r.Read(buffer)
}