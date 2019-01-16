package awsapi
/*
import "io"

type Reader2Writer struct {
	buffer    chan []byte // data channel
	error     chan error  // return the error from the background func
	readFunc  ReadFunc   // the read function works in the background, taking the writer as a parameter and reading the data from S3
	inited    bool        // on the first call to Read, readFunc is called in background, getting it's input using the buffer chan
	leftOvers []byte      // when write buffer is smaller than read buffer, we need this to keep the left overs until next write. Hopefully not too big ...
}

type ReadFunc func(writer io.WriterAt) (int64, error)


func NewReader2Writer(readFunc ReadFunc) *Reader2Writer {
	return &Reader2Writer{
		buffer:    make(chan []byte),
		error:     make(chan error),
		readFunc: readFunc,
		inited:    false,
	}
}

//func (w *Reader2Writer) Read(buffer []byte) (n int, err error) {

func (w *Reader2Writer) Read(buffer []byte) (n int, err error) {
	// on first call to Read open a new channel to help sending the next calls to Reader.Read into Write.WriteAt
	if !w.inited {
		w.inited = true
		go backgroundReader(w)
	}

	return readFromBackground(w, buffer), nil
}

func (r *Reader2Writer) Write(bytes []byte) (int, error) {
	// on first call to Write open a new channel to help sending the next calls to Writer.Write into Reader.Read
	if !r.inited {
		r.inited = true
		go backgroundReader(r)
	}

	sendToReader(r, bytes)

	// Write will never return an error. the error is returned upon a call to Close
	return len(bytes), nil
}

func readFromBackground(w *Reader2Writer, buffer []byte) int {
	// channel might be closed due to error in readFunc, just recover (the error is "can't send to a closed channel")
	defer func() {
		recover()
	}()

	w.buffer <- buffer
}

func backgroundReader(r *Reader2Writer) {
	err := r.writeFunc(r)
	r.closeBufferSafe()

	r.error <- err
}

func (r *Reader2Writer) Close() error {
	r.closeBufferSafe()

	// wait for completion
	err := <- r.error

	return err
}

func (r *Reader2Writer) closeBufferSafe() {
	// channel might be closed due to error in writeFunc, just recover (the error is "close of a closed channel")
	defer func() {
		recover()
	}()

	close(r.buffer)
}

*/






















/*
func (r *Reader2Writer) Read(p []byte) (int, error) {
	panic ("AbugovTODO")
	n, err := r.readWithRetry(p)
		if r.remain != -1 {
			r.remain -= int64(n)
		}
		if r.checkCRC {
			r.gotCRC = crc32.Update(r.gotCRC, crc32cTable, p[:n])
			// Check CRC here. It would be natural to check it in Close, but
			// everybody defers Close on the assumption that it doesn't return
			// anything worth looking at.
			if r.remain == 0 { // Only check if we have Content-Length.
				r.checkedCRC = true
				if r.gotCRC != r.wantCRC {
					return n, fmt.Errorf("storage: bad CRC on read: got %d, want %d",
						r.gotCRC, r.wantCRC)
				}
			}
		}
		return n, err
}

func (r *Reader2Writer) readWithRetry(p []byte) (int, error) {
	panic ("AbugovTODO")
	n := 0
		for len(p[n:]) > 0 {
			m, err := r.body.Read(p[n:])
			n += m
			r.seen += int64(m)
			if !shouldRetryRead(err) {
				return n, err
			}
			// Read failed, but we will try again. Send a ranged read request that takes
			// into account the number of bytes we've already seen.
			res, err := r.reopen(r.seen)
			if err != nil {
				// reopen already retries
				return n, err
			}
			r.body.Close()
			r.body = res.Body
		}
		return n, nil
}

func (r *Reader2Writer) Close() error {
	n, err := r.readWithRetry(p)
	if r.remain != -1 {
		r.remain -= int64(n)
	}
	if r.checkCRC {
		r.gotCRC = crc32.Update(r.gotCRC, crc32cTable, p[:n])
		// Check CRC here. It would be natural to check it in Close, but
		// everybody defers Close on the assumption that it doesn't return
		// anything worth looking at.
		if r.remain == 0 { // Only check if we have Content-Length.
			r.checkedCRC = true
			if r.gotCRC != r.wantCRC {
				return n, fmt.Errorf("storage: bad CRC on read: got %d, want %d",
					r.gotCRC, r.wantCRC)
			}
		}
	}
	return n, err
}

func shouldRetryRead(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasSuffix(err.Error(), "INTERNAL_ERROR") && strings.Contains(reflect.TypeOf(err).String(), "http2")
}*/