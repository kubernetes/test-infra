package awsapi

import (
	"io"
)

// Writer2Reader implements both Writer and Reader interfaces.
// It's used as a wrapper around S3Upload so we can write to S3 using io.Copy.
// When Reader.Read is called, it is blocking until the first Writer.Write call.
// The following Reader.Read operations are blocked until Writer is closed or Writer.Write is called again.
// Flow:
// io.Copy reads the source and writes the data to Writer2Reader.Write.
// The data is sent to the background using the buffer chan to S3Upload.
// S3Upload is calling Writer2Reader.Read which reads the data from the buffer channel

type Writer2Reader struct {
	buffer    chan []byte // data channel
	error     chan error  // return the error from the background func
	writeFunc WriteFunc   // the write function works in the background, taking the reader as a parameter and writing the data into S3
	inited    bool        // on the first call to Write, writeFunc is called in background, getting it's input using the buffer chan
	leftOvers []byte      // when read buffer is smaller than write buffer, we need this to keep the left overs until next read. Hopefully not too big ...
}

type WriteFunc func(reader io.Reader) error

func NewWriter2Reader(writeFunc WriteFunc) *Writer2Reader {
	return &Writer2Reader{
		buffer:    make(chan []byte),
		error:     make(chan error),
		writeFunc: writeFunc,
		inited:    false,
	}
}

func (w *Writer2Reader) Read(buffer []byte) (n int, err error) {
	size := 0

	if len(w.leftOvers) > 0 {
		size = copy(buffer, w.leftOvers)
		w.leftOvers = w.leftOvers[size:]

		if len(buffer) - size == 0 {
			return len(buffer), nil
		}
	}

	// read or block
	buf, ok := <-w.buffer

	if ok {

		size = copy(buffer, buf)

		if size < len(buf) {
			w.leftOvers = buf[size:]
		}

		return size, nil
	} else {
		return size, io.EOF
	}
}

func (w *Writer2Reader) Write(bytes []byte) (int, error) {
	// on first call to Write open a new channel to help sending the next calls to Writer.Write into Reader.Read
	if !w.inited {
		w.inited = true
		go backgroundWriter(w)
	}

	sendToWriter(w, bytes)

	// Write will never return an error. the error is returned upon a call to Close
	return len(bytes), nil
}

func sendToWriter(w *Writer2Reader, bytes []byte) {
	// must copy before send otherwise the caller of this function can change the content just before read on the other side
	c := make([]byte, len(bytes))
	copy(c, bytes)

	// channel might be closed due to error in writeFunc, just recover (the error is "can't send to a closed channel")
	defer func() {
		recover()
	}()

	w.buffer <- c
}

func backgroundWriter(w *Writer2Reader) {
	err := w.writeFunc(w)
	w.closeBufferSafe()

	w.error <- err
}

func (w *Writer2Reader) Close() error {
	w.closeBufferSafe()

	// wait for completion
	err := <- w.error

	return err
}

func (w *Writer2Reader) closeBufferSafe() {
	// channel might be closed due to error in writeFunc, just recover (the error is "close of a closed channel")
	defer func() {
		recover()
	}()

	close(w.buffer)
}