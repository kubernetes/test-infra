package awsapi

import (
	"context"
	"github.com/stretchr/testify/assert"
	"fmt"
	"testing"
)

func Test_S3DownloadWithRangeReader(t *testing.T) {
	opt := ClientOption {
		CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(opt)
	assert.NoError(t, err)

	b := client.Bucket("okro-prow-test")
	o := b.Object("f10m.txt")

	size := int64(100000)
	offset := int64(10000)

	reader := o.NewRangeReader(context.Background(), offset, size)

	buf := make([]byte, size)

	n, err := reader.Read(buf)
	assert.NoError(t, err)

	assert.Equal(t, size, n)
	assert.NotEqual(t, make([]byte, size), buf)
}

func Test_S3Download(t *testing.T) {
	opt := ClientOption {
		CredentialsFile: "/users/Traiana/alexa/.aws/credentials",
	}

	client, err := NewClient(opt)
	assert.NoError(t, err)

	s := int64(100000)

	b := client.Bucket("okro-prow-test")
	m := MyTestWriter{ make([]byte, s)}

	n, err := S3Download(m, b, "f10m.txt", 10000, s)
	assert.NoError(t, err)
	assert.Equal(t, s, n)
	assert.NotEqual(t, make([]byte, s), m.buffer)

}

type MyTestWriter struct {
	buffer []byte
}

func (m MyTestWriter) WriteAt(p []byte, offset int64) (n int, err error) {

	if int64(len(p)) + offset > int64(len(m.buffer)) {
		panic(fmt.Sprintf("buffer size (%v) not big enough (offset: %v, len: %v)", len(m.buffer), offset, len(p)))
	}

	return copy(m.buffer, p), nil
}