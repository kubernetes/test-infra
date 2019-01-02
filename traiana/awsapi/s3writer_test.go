package awsapi

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

func Test_S3Writer(t *testing.T) {
	client, err := NewClient()

	s3Writer := &S3Writer{
		handle: Bucket("dev-okro-io", client),
		key: "lala",
	}

	/*dat, err := ioutil.ReadFile("/Users/Traiana/alexa/file.txt")
	rs := strings.NewReader(string(dat))
	io.Copy(s3Writer, rs)

	if err != nil {
	}*/

	file, err := os.Open("/Users/Traiana/alexa/file.txt")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer file.Close()

	io.Copy(s3Writer, &SlowReader{file})
	s3Writer.Close()
}

// simulate slow read, to let S3 upload wait a while
type SlowReader struct {
	r *os.File
}

func (s *SlowReader) Read(buffer []byte) (n int, err error) {
	time.Sleep(100 * time.Millisecond)
	return s.r.Read(buffer)
}