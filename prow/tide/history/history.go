/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package history provides an append only, size limited log of recent actions
// that Tide has taken for each subpool.
package history

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/io"
)

// Mock out time for unit testing.
var now = time.Now

// History uses a `*recordLog` per pool to store a record of recent actions that
// Tide has taken. Using a log per pool ensure that history is retained
// for inactive pools even if other pools are very active.
type History struct {
	logs map[string]*recordLog
	sync.Mutex
	logSizeLimit int

	opener opener
	path   string
}

// opener has methods to read and write paths
type opener interface {
	Reader(ctx context.Context, path string) (io.ReadCloser, error)
	Writer(ctx context.Context, path string, opts ...io.WriterOptions) (io.WriteCloser, error)
}

func readHistory(maxRecordsPerKey int, opener opener, path string) (map[string]*recordLog, error) {
	reader, err := opener.Reader(context.Background(), path)
	if io.IsNotExist(err) { // No history exists yet. This is not an error.
		return map[string]*recordLog{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open: %v", err)
	}
	defer io.LogClose(reader)
	raw, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read: %v", err)
	}
	var recordsByPool map[string][]*Record
	if err := json.Unmarshal(raw, &recordsByPool); err != nil {
		return nil, fmt.Errorf("unmarshal: %v", err)
	}

	// Load records into a new recordLog map.
	logsByPool := make(map[string]*recordLog, len(recordsByPool))
	for poolKey, records := range recordsByPool {
		logsByPool[poolKey] = newRecordLog(maxRecordsPerKey)
		limit := maxRecordsPerKey
		if len(records) < limit {
			limit = len(records)
		}
		for i := limit - 1; i >= 0; i-- {
			logsByPool[poolKey].add(records[i])
		}
	}
	return logsByPool, nil
}

func writeHistory(opener opener, path string, hist map[string][]*Record) error {
	// a write's duration will scale with the volume of data to write but large
	// data sets can finish in about 500ms; a timeout of 30s should not evict
	// well-behaved writes
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	writer, err := opener.Writer(ctx, path)
	if err != nil {
		return fmt.Errorf("open: %v", err)
	}
	b, err := json.Marshal(hist)
	if err != nil {
		return fmt.Errorf("marshal: %v", err)
	}
	if _, err := fmt.Fprint(writer, string(b)); err != nil {
		io.LogClose(writer)
		return fmt.Errorf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close: %v", err)
	}
	return nil
}

// Record is an entry describing one action that Tide has taken (e.g. TRIGGER or MERGE).
type Record struct {
	Time    time.Time      `json:"time"`
	Action  string         `json:"action"`
	BaseSHA string         `json:"baseSHA,omitempty"`
	Target  []prowapi.Pull `json:"target,omitempty"`
	Err     string         `json:"err,omitempty"`
}

// New creates a new History struct with the specificed recordLog size limit.
func New(maxRecordsPerKey int, opener io.Opener, path string) (*History, error) {
	hist := &History{
		logs:         map[string]*recordLog{},
		logSizeLimit: maxRecordsPerKey,
		opener:       opener,
		path:         path,
	}

	if path != "" {
		// Load existing history from GCS.
		var err error
		start := time.Now()
		hist.logs, err = readHistory(maxRecordsPerKey, hist.opener, hist.path)
		if err != nil {
			return nil, err
		}
		logrus.WithFields(logrus.Fields{
			"duration": time.Since(start).String(),
			"path":     hist.path,
		}).Debugf("Successfully read action history for %d pools.", len(hist.logs))
	}

	return hist, nil
}

// Record appends an entry to the recordlog specified by the poolKey.
func (h *History) Record(poolKey, action, baseSHA, err string, targets []prowapi.Pull) {
	t := now()
	sort.Sort(ByNum(targets))
	h.addRecord(
		poolKey,
		&Record{
			Time:    t,
			Action:  action,
			BaseSHA: baseSHA,
			Target:  targets,
			Err:     err,
		},
	)
}

func (h *History) addRecord(poolKey string, rec *Record) {
	h.Lock()
	defer h.Unlock()
	if _, ok := h.logs[poolKey]; !ok {
		h.logs[poolKey] = newRecordLog(h.logSizeLimit)
	}
	h.logs[poolKey].add(rec)
}

// ServeHTTP serves a JSON mapping from pool key -> sorted records for the pool.
func (h *History) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, err := json.Marshal(h.AllRecords())
	if err != nil {
		logrus.WithError(err).Error("Encoding JSON history.")
		b = []byte("{}")
	}
	if _, err = w.Write(b); err != nil {
		logrus.WithError(err).Debug("Writing JSON history response.")
	}
}

// Flush writes the action history to persistent storage if configured to do so.
func (h *History) Flush() {
	if h.path == "" {
		return
	}
	records := h.AllRecords()
	start := time.Now()
	err := writeHistory(h.opener, h.path, records)
	log := logrus.WithFields(logrus.Fields{
		"duration": time.Since(start).String(),
		"path":     h.path,
	})
	if err != nil {
		log.WithError(err).Error("Error flushing action history to GCS.")
	} else {
		log.Debugf("Successfully flushed action history for %d pools.", len(h.logs))
	}
}

// AllRecords generates a map from pool key -> sorted records for the pool.
func (h *History) AllRecords() map[string][]*Record {
	h.Lock()
	defer h.Unlock()

	res := make(map[string][]*Record, len(h.logs))
	for key, log := range h.logs {
		res[key] = log.toSlice()
	}
	return res
}

// recordLog is a space efficient, limited size, append only list.
type recordLog struct {
	buff  []*Record
	head  int
	limit int

	// cachedSlice is the cached, in-order slice. Use toSlice(), don't access directly.
	// We cache this value because most pools don't change between sync loops.
	cachedSlice []*Record
}

func newRecordLog(sizeLimit int) *recordLog {
	return &recordLog{
		head:  -1,
		limit: sizeLimit,
	}
}

func (rl *recordLog) add(rec *Record) {
	// Start by invalidating cached slice.
	rl.cachedSlice = nil

	rl.head = (rl.head + 1) % rl.limit
	if len(rl.buff) < rl.limit {
		// The log is not yet full. Append the record.
		rl.buff = append(rl.buff, rec)
	} else {
		// The log is full. Overwrite the oldest record.
		rl.buff[rl.head] = rec
	}
}

func (rl *recordLog) toSlice() []*Record {
	if rl.cachedSlice != nil {
		return rl.cachedSlice
	}

	res := make([]*Record, 0, len(rl.buff))
	for i := 0; i < len(rl.buff); i++ {
		index := (rl.limit + rl.head - i) % rl.limit
		res = append(res, rl.buff[index])
	}
	rl.cachedSlice = res
	return res
}

// ByNum implements sort.Interface for []PRMeta to sort by ascending PR number.
type ByNum []prowapi.Pull

func (prs ByNum) Len() int           { return len(prs) }
func (prs ByNum) Swap(i, j int)      { prs[i], prs[j] = prs[j], prs[i] }
func (prs ByNum) Less(i, j int) bool { return prs[i].Number < prs[j].Number }
