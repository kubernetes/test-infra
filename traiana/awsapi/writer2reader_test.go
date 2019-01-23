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
// 2. s3Upload read buffer size

func TestWriter2Reader(t *testing.T) {
	tests := []struct {
		name                        string
		input                       []byte
		ioCopyBufSize, s3UploadBufSize int
	}{
		{
			name:          "equal buffer size, both buffers too small",
			input:         []byte{1, 2},
			ioCopyBufSize: 1,
			s3UploadBufSize:  1,
		},
		{
			name:          "equal buffer size, both buffers big enough",
			input:         []byte{1, 2},
			ioCopyBufSize: 3,
			s3UploadBufSize:  3,
		},
		{
			name:          "equal buffer size, both buffers are equal to data size",
			input:         []byte{1, 2},
			ioCopyBufSize: 2,
			s3UploadBufSize:  2,
		},
		{
			name:          "IoCopy bigger, both buffers big enough",
			input:         []byte{1, 2},
			ioCopyBufSize: 4,
			s3UploadBufSize:  3,
		},
		{
			name:          "IoCopy bigger, both buffers too small",
			input:         []byte{1, 2, 3},
			ioCopyBufSize: 2,
			s3UploadBufSize:  1,
		},
		{
			name:          "IoCopy bigger, 3sPut too small",
			input:         []byte{1, 2},
			ioCopyBufSize: 2,
			s3UploadBufSize:  1,
		},
		{
			name:          "IoCopy bigger, IoCopy bigger than data and S3Upload too small",
			input:         []byte{1, 2},
			ioCopyBufSize: 3,
			s3UploadBufSize:  1,
		},
		{
			name:          "S3Upload bigger, both buffers big enough",
			input:         []byte{1, 2},
			ioCopyBufSize: 3,
			s3UploadBufSize:  4,
		},
		{
			name:          "S3Upload bigger, both buffers too small",
			input:         []byte{1, 2, 3},
			ioCopyBufSize: 1,
			s3UploadBufSize:  2,
		},
		{
			name:          "S3Upload bigger, IoCopy too small",
			input:         []byte{1, 2},
			ioCopyBufSize: 1,
			s3UploadBufSize:  2,
		},
		{
			name:          "S3Upload bigger, S3Upload bigger than data and IoCopy too small",
			input:         []byte{1, 2},
			ioCopyBufSize: 1,
			s3UploadBufSize:  3,
		},
	}
	for _, tt := range tests {
		output := doReadWrite(tt.input,tt.ioCopyBufSize,tt.s3UploadBufSize)
		msg := fmt.Sprintf("(input size: %v, io.Copy read buffer size: %v, s3Upload read buffer size: %v)", len(tt.input), tt.ioCopyBufSize, tt.s3UploadBufSize)
		assert.Equal(t, tt.input, output, msg)
	}
}

func Test_WriteError(t *testing.T) {
	const loops = 3 // times to loop before error

	bg := func(wr *Writer2Reader) error {
		for i := 0; i < loops-1; i++ {
			wr.Write([]byte {1})
		}

		return errors.New("write error!")
	}

	wr := NewWriter2Reader(bg)
	var err error

	for i := 0; i < loops; i++ {
		b := []byte{0}
		_, err = wr.Read(b)

		if err != nil {
			break
		}
	}

	assert.Error(t, err)
	assert.NotEqual(t, io.EOF, err)
}

func Test_ReadError(t *testing.T) {
	const loops = 3 // times to loop before error

	b := make([]byte, 1)

	bg := func(wr *Writer2Reader) error {
		for i := 0; i < loops-1; i++ {
			wr.Read(b)
		}

		return errors.New("read error!")
	}

	wr := NewWriter2Reader(bg)

	for i := 0; i < loops; i++ {
		wr.Write([]byte{1})
	}

	err := wr.Close()

	assert.Error(t, err)
	assert.NotEqual(t, io.EOF, err)
}

func doReadWrite(input []byte, ioCopyBufSize int, s3UploadBufSize int) ([]byte) {
	var output []byte

	// mock for S3Upload, which takes a reader as input (see UploadInput.Body in aws-sdk-go/service/s3/s3manager/upload.go)
	bg := func(wr *Writer2Reader) error {
		buffer := make([]byte, s3UploadBufSize)

		for {
			n, err := wr.Read(buffer)

			if n > 0 {
				output = append(output, buffer[0:n]...)
			}

			if err == io.EOF {
				break
			}
		}

		return nil
	}

	target := NewWriter2Reader(bg)

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

type Write func([]byte) (int, error)

type MyWriter struct {
	write Write
}

func (r MyWriter) Write(p []byte) (n int, err error) {
	return r.write(p)
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
