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





func TestWriter2Reader(t *testing.T) {
	tests := []struct {
		name                        string
		input                       []byte
		ioCopyBufSize, s3PutBufSize int
	}{
		{
			name:          "equal buffer size, both buffers too small",
			input:         []byte{1, 2},
			ioCopyBufSize: 1,
			s3PutBufSize:  1,
		},
		{
			name:          "equal buffer size, both buffers big enough",
			input:         []byte{1, 2},
			ioCopyBufSize: 3,
			s3PutBufSize:  3,
		},
		{
			name:          "equal buffer size, both buffers are equal to data size",
			input:         []byte{1, 2},
			ioCopyBufSize: 2,
			s3PutBufSize:  2,
		},
		{
			name:          "IoCopy bigger, both buffers big enough",
			input:         []byte{1, 2},
			ioCopyBufSize: 4,
			s3PutBufSize:  3,
		},
		{
			name:          "IoCopy bigger, both buffers too small",
			input:         []byte{1, 2, 3},
			ioCopyBufSize: 2,
			s3PutBufSize:  1,
		},
		{
			name:          "IoCopy bigger, 3sPut too small",
			input:         []byte{1, 2},
			ioCopyBufSize: 2,
			s3PutBufSize:  1,
		},
		{
			name:          "IoCopy bigger, IoCopy bigger than data and S3Put too small",
			input:         []byte{1, 2},
			ioCopyBufSize: 3,
			s3PutBufSize:  1,
		},
		{
			name:          "S3Put bigger, both buffers big enough",
			input:         []byte{1, 2},
			ioCopyBufSize: 3,
			s3PutBufSize:  4,
		},
		{
			name:          "S3Put bigger, both buffers too small",
			input:         []byte{1, 2, 3},
			ioCopyBufSize: 1,
			s3PutBufSize:  2,
		},
		{
			name:          "S3Put bigger, IoCopy too small",
			input:         []byte{1, 2},
			ioCopyBufSize: 1,
			s3PutBufSize:  2,
		},
		{
			name:          "S3Put bigger, S3Put bigger than dataand IoCopy too small",
			input:         []byte{1, 2},
			ioCopyBufSize: 1,
			s3PutBufSize:  3,
		},
	}
	for _, tt := range tests {
		output := doReadWrite(tt.input,tt.ioCopyBufSize,tt.s3PutBufSize)
		msg := fmt.Sprintf("(input size: %v, io.Copy read buffer size: %v, s3Put read buffer size: %v)", len(tt.input), tt.ioCopyBufSize, tt.s3PutBufSize)
		assert.Equal(t, tt.input, output, msg)
	}
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

func doReadWrite(input []byte, ioCopyBufSize int, s3PutBufSize int) ([]byte) {
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

	return  output
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
