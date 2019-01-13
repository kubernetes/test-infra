package awsapi

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"errors"
	"testing"
)

// There r 2 critical buffer sizes that we test here:
// 1. io.Copy read buffer size
// 2. s3Put read buffer size

func Test_Writer2Reader_EqualBufferSize_BothBuffersTooSmall(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2},1,1)
	assert.Equal(t, output, input, msg)
}

func Test_Writer2Reader_EqualBufferSize_BothBuffersBigEnough(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2},3,3)
	assert.Equal(t, output, input, msg)
}

func Test_Writer2Reader_EqualBufferSize_BothBuffersAreEqualToDataSize(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2},3,3)
	assert.Equal(t, output, input, msg)
}

func Test_Writer2Reader_IoCopyBigger_BothBuffersBigEnough(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2},4,3)
	assert.Equal(t, output, input, msg)
}

func Test_Writer2Reader_IoCopyBigger_BothBuffersTooSmall(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2,3},2,1)
	assert.Equal(t, output, input, msg)
}

func Test_Writer2Reader_IoCopyBigger_3sPutTooSmall(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2},2,1)
	assert.Equal(t, output, input, msg)
}

func Test_Writer2Reader_IoCopyBigger_IoCopyBiggerThanData_S3PutTooSmall(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2},3,1)
	assert.Equal(t, output, input, msg)
}

func Test_Writer2Reader_S3PutBigger_BothBuffersBigEnough(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2},3,4)
	assert.Equal(t, output, input, msg)
}

func Test_Writer2Reader_S3PutBigger_BothBuffersTooSmall(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2,3},1,2)
	assert.Equal(t, output, input, msg)
}

func Test_Writer2Reader_S3PutBigger_IoCopyTooSmall(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2},1,2)
	assert.Equal(t, output, input, msg)
}

func Test_Writer2Reader_S3PutBigger_S3PutBiggerThanData_IoCopyTooSmall(t *testing.T) {
	input, output, msg := doReadWrite([]byte{1,2},1,3)
	assert.Equal(t, output, input, msg)
}

func Test_WriteError(t *testing.T) {
	const sendCount = 3 // send data in 3 chunks
	const getCount = 2  // get 2 chunks before returning an error

	b := make([]byte, 1)

	writeFunc := func(reader io.Reader) error {
		for i := 0; i < getCount; i++ {
			reader.Read(b)
		}

		return errors.New("write error!")
	}

	target := NewWriter2Reader(writeFunc)

	reads := 0

	source := MyReader{
		read: func(bytes []byte) (int, error) {
			reads += 1

			if reads == sendCount {
				return 1, io.EOF
			}

			return 1, nil
		},
	}

	myIoCopy(1, target, source)
	err := target.Close()

	assert.Error(t, err)
}

func doReadWrite(input []byte, ioCopyBufSize int, s3PutBufSize int) ([]byte, []byte, string) {
	var output []byte

	// mock for S3Put, which takes a reader as input (see UploadInput.Body in aws-sdk-go/service/s3/s3manager/upload.go)
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

	// don't use io.Copy so we can control the buffer size
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

// Same logic as io.Copy
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
