package awsapi

import (
	"io"
)

// Writer2Reader implements both Writer and Reader interfaces.
// This class solves the problem of -
// 1. prow downloads files using a reader, but S3Download takes a writer as input
// 2. prow uploads files using a writer, but S3Upload takes a reader as input
//
// When Reader.Read is called, it is blocked until the next Writer.Write call.
//
// Flow:
// S3Upload is called in the background, calling Writer2Reader.Read which is blocked.
// io.Copy reads the source bytes and sends them to Writer2Reader.Write.
// The data is sent to the background using the data chan.
// Writer2Reader.Read continues.

type Writer2Reader struct {
	data      chan []byte           // data channel
	error     chan error            // return the error from the background func
	bgWorker  Writer2ReaderBgWorker // this function is run in the background, taking the Writer2Reader as param and waiting for input from the other side
	leftOvers []byte                // when read data is smaller than write data, we need this to keep the left overs until next read. Hopefully not too big ...
}

type Writer2ReaderBgWorker func(wr *Writer2Reader) error

func NewWriter2Reader(bgWorker Writer2ReaderBgWorker) *Writer2Reader {
	wr := &Writer2Reader{
		data:     make(chan []byte),
		error:    make(chan error),
		bgWorker: bgWorker,
	}

	bg := func(wr *Writer2Reader) {
		err := wr.bgWorker(wr)
		wr.closeDataChanSafe()
		wr.error <- err
	}

	go bg(wr)

	return wr
}

func (w *Writer2Reader) Read(data []byte) (n int, err error) {
	size := 0

	if len(w.leftOvers) > 0 {
		size = copy(data, w.leftOvers)
		w.leftOvers = w.leftOvers[size:]

		if len(data) - size == 0 {
			return len(data), nil
		}
	}

	// read or block
	buf, ok := <-w.data

	if ok {
		size = copy(data, buf)

		if size < len(buf) {
			w.leftOvers = buf[size:]
		}

		return size, nil
	} else {
		return size, io.EOF
	}
}

func (w *Writer2Reader) Write(bytes []byte) (int, error) {
	sendToReader(w, bytes)

	// Write will never return an error. the error is returned upon a call to Close
	return len(bytes), nil
}

func (w *Writer2Reader) WriteAt(bytes []byte, offset int64) (n int, err error) {
	// offset is ignored since we write sync and incremental

	sendToReader(w, bytes)

	// Write will never return an error. the error is returned upon a call to Close
	return len(bytes), nil
}

func sendToReader(w *Writer2Reader, bytes []byte) {
	// must copy before send otherwise the caller of this function can change the content just before read on the other side
	c := make([]byte, len(bytes))
	copy(c, bytes)

	// channel might be closed due to error in writeFunc, just recover (the error is "can't send to a closed channel")
	defer func() {
		recover()
	}()

	w.data <- c
}

func (w *Writer2Reader) Close() error {
	w.closeDataChanSafe()

	// wait for completion
	err := <- w.error

	return err
}

func (w *Writer2Reader) closeDataChanSafe() {
	// channel might be closed due to error in writeFunc, just recover (the error is "close of a closed channel")
	defer func() {
		recover()
	}()

	close(w.data)
}