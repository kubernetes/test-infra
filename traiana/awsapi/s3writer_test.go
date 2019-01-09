package awsapi

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

func Test_S3Writer(t *testing.T) {
	o := ClientOption {
			CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(o)

	s3Writer := &S3Writer{
			handle: Bucket("dev-okro-io", client),
			key: "lala",
	}

	file, err := os.Open("/Users/Traiana/alexa/Downloads/f1.txt")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer file.Close()

	io.Copy(s3Writer, &SlowReader{file})
	s3Writer.Close()
}

/*func Test_S3Put(t *testing.T) {
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