package awsapi

import (
	"github.com/magiconair/properties/assert"
	"io"
	"k8s.io/test-infra/bazel-test-infra/external/go_sdk/src/fmt"
	"testing"
)

// TODO:               ________________ TEST ERRORs during write


// There r 2 critical buffer sizes that we test here:
// 1. io.Copy read buffer size
// 2. s3Put read buffer size

func Test_Writer2Reader_EqualBufferSize_BothBuffersTooSmall(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2},1,1)
	assert.Equal(t, output, input, stats)
}

func Test_Writer2Reader_EqualBufferSize_BothBuffersBigEnough(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2},3,3)
	assert.Equal(t, output, input, stats)
}

func Test_Writer2Reader_EqualBufferSize_BothBuffersAreEqualToDataSize(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2},3,3)
	assert.Equal(t, output, input, stats)
}

func Test_Writer2Reader_IoCopyBigger_BothBuffersBigEnough(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2},4,3)
	assert.Equal(t, output, input, stats)
}

func Test_Writer2Reader_IoCopyBigger_BothBuffersTooSmall(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2,3},2,1)
	assert.Equal(t, output, input, stats)
}

func Test_Writer2Reader_IoCopyBigger_3sPutTooSmall(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2},2,1)
	assert.Equal(t, output, input, stats)
}

func Test_Writer2Reader_IoCopyBigger_IoCopyBiggerThanData_S3PutTooSmall(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2},3,1)
	assert.Equal(t, output, input, stats)
}

func Test_Writer2Reader_S3PutBigger_BothBuffersBigEnough(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2},3,4)
	assert.Equal(t, output, input, stats)
}

func Test_Writer2Reader_S3PutBigger_BothBuffersTooSmall(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2,3},1,2)
	assert.Equal(t, output, input, stats)
}

func Test_Writer2Reader_S3PutBigger_IoCopyTooSmall(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2},1,2)
	assert.Equal(t, output, input, stats)
}

func Test_Writer2Reader_S3PutBigger_S3PutBiggerThanData_IoCopyTooSmall(t *testing.T) {
	input, output, stats := doReadWrite([]byte{1,2},1,3)
	assert.Equal(t, output, input, stats)
}

func doReadWrite(input []byte, ioCopyBufSize int, s3PutBufSize int) ([]byte, []byte, string) {
	var output []byte

	writeFunc := func(reader io.Reader) error {
		buffer := make([]byte, s3PutBufSize)

		for {
			n, err := reader.Read(buffer)

			if n > 0 {
				output = append(output, buffer[0:n]...)
			}

			if err == io.EOF {
				break
			}
		}

		return nil

		//~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
		//return io.EOF
	}

	target := NewWriter2Reader(writeFunc)

	l := 0

	source := MyReader{
		read: func(bytes []byte) (int, error) {
			n := copy(bytes, input[l:])
			l += n

			if l >= len(input) {
				return n, io.EOF
			}

			return n, nil
		},
	}

	myIoCopy(ioCopyBufSize, target, source)
	target.Close()

	return input, output, fmt.Sprintf("(input size: %v, io.Copy read buffer size: %v, s3Put read buffer size: %v)", len(input), ioCopyBufSize, s3PutBufSize)
}

type Read func([]byte) (int, error)

type MyReader struct {
	read Read
}

func (r MyReader) Read(p []byte) (n int, err error) {
	return r.read(p)
}

func myIoCopy(bufSize int, dst io.Writer, src io.Reader) (written int64, err error) {
	buf := make([]byte, bufSize)

	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}

	return written, err
}

