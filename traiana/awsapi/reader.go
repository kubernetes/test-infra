package awsapi

import (
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"reflect"
	"strings"
)

var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

type Reader struct {
	body               io.ReadCloser
	seen, remain, size int64
	contentType        string
	contentEncoding    string
	cacheControl       string
	checkCRC           bool   // should we check the CRC?
	wantCRC            uint32 // the CRC32c value the server sent in the header
	gotCRC             uint32 // running crc
	checkedCRC         bool   // did we check the CRC? (For tests.)
	reopen             func(seen int64) (*http.Response, error)
}

func (r Reader) Read(p []byte) (int, error) {
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
func (r *Reader) readWithRetry(p []byte) (int, error) {
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
func (r *Reader) Close() error {
	panic ("AbugovTODO")
/*	n, err := r.readWithRetry(p)
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
	return n, err*/
}

//AbugovTODO
func shouldRetryRead(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasSuffix(err.Error(), "INTERNAL_ERROR") && strings.Contains(reflect.TypeOf(err).String(), "http2")
}