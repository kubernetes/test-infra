package awsapi

import (
	"io"
	"k8s.io/test-infra/bazel-test-infra/external/go_sdk/src/context"
	"os"
	"testing"
	"time"
)

func Test_S3Put(t *testing.T) {
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

/*func Test_S3Put___(t *testing.T) {
	o := ClientOption {
		CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(o)

	file, err := os.Open("/Users/Traiana/alexa/Downloads/f1.txt")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer file.Close()

	err = s3Put(&SlowReader{file}, Bucket("dev-okro-io", client), "lala")
}*/

/*func Test_FileWriter(t *testing.T) {
	source, err := os.Open("/Users/Traiana/alexa/Downloads/f1.txt")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer source.Close()

	target, err := os.Create("/Users/Traiana/alexa/Downloads/fw.txt")
	defer target.Close()

	w := bufio.NewWriter(target)

	io.Copy(w, &SlowReader{source})
	w.Flush()
}*/

// simulate slow read, to let S3 upload wait a while
type SlowReader struct {
	r *os.File
}

func (s *SlowReader) Read(buffer []byte) (n int, err error) {
	time.Sleep(100 * time.Millisecond)
	return s.r.Read(buffer)
}