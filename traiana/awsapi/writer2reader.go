package awsapi

import (
	"io"
	"k8s.io/test-infra/bazel-test-infra/external/go_sdk/src/fmt"
)

// Writer2Reader implements both Writer and Reader interfaces.
// It's used as a wrapper around S3Put so we can write to S3 using io.Copy.
// When Reader.Read is called, it is blocking until the first Writer.Write call.
// The following Reader.Read operations are blocked until Writer is closed or Writer.Write is called again.

type Writer2Reader struct {
	buffer chan []byte
	error chan error
	writeFunc WriteFunc
	firstWrite bool // on the first call to Write writeFunc is called in background, reading it's input using the buffer chan
	leftOvers []byte
}

type WriteFunc func(reader io.Reader) error

func NewWriter2Reader(writeFunc WriteFunc) *Writer2Reader {
	return &Writer2Reader{
		buffer: make(chan []byte),
		error: make(chan error),
		writeFunc: writeFunc,
		firstWrite: true,
	}
}

func (wr *Writer2Reader) Read(buffer []byte) (n int, err error) {
	spaceLeft := len(buffer)
	size := 0

	if len(wr.leftOvers) > 0 {
		size = copy(buffer, wr.leftOvers)
		wr.leftOvers = wr.leftOvers[size:]
		spaceLeft -= size
	}

	if spaceLeft == 0 {
		return len(buffer), nil
	}

	// read or block
	//time.Sleep(time.Second)

	buf, ok := <-wr.buffer
	fmt.Println("gotA: ", buf, ok)

	//buf1, ok1 := <-wr.buffer
	//fmt.Println("gotB: ", buf1, ok1)
	//time.Sleep(time.Second)

	if ok {
		size = copy(buffer, buf)

		if size < len(buf) {
			wr.leftOvers = buf[size:]
		}

		return size, nil
	} else {
		return size, io.EOF
	}
}

func (wr *Writer2Reader) Write(bytes []byte) (n int, err error) {
	// on first call to Write open a new channel to help sending the next calls to Writer.Write into Reader.Read
	if wr.firstWrite {
		wr.firstWrite = false
		go backgroundWriter(wr)
	}
	fmt.Println("sending: ", bytes)

	wr.buffer <- bytes
	fmt.Println("sent: ", bytes)


	// Write will never return an error. the error is returned upon a call to Close
	return len(bytes), nil
}

func backgroundWriter(wr *Writer2Reader) {
	err := wr.writeFunc(wr)
	wr.error <- err
}

func (wr Writer2Reader) Close() error {
	close(wr.buffer)

	// wait for completion
	err := <- wr.error

	return err
}