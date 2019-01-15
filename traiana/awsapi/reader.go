package awsapi

import (
	"io"
	"net/http"
)

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