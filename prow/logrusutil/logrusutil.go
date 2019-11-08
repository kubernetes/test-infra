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

// Package logrusutil implements some helpers for using logrus
package logrusutil

import (
	"bytes"
	"crypto/md5"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
)

// DefaultFieldsFormatter wraps another logrus.Formatter, injecting
// DefaultFields into each Format() call, existing fields are preserved
// if they have the same key
type DefaultFieldsFormatter struct {
	WrappedFormatter logrus.Formatter
	DefaultFields    logrus.Fields
	PrintLineNumber  bool
}

// Init set Logrus formatter
// if DefaultFieldsFormatter.wrappedFormatter is nil &logrus.JSONFormatter{} will be used instead
func Init(formatter *DefaultFieldsFormatter) {
	if formatter == nil {
		return
	}
	if formatter.WrappedFormatter == nil {
		formatter.WrappedFormatter = &logrus.JSONFormatter{}
	}
	logrus.SetFormatter(formatter)
	logrus.SetReportCaller(formatter.PrintLineNumber)
}

// ComponentInit is a syntax sugar for easier Init
func ComponentInit(component string) {
	Init(
		&DefaultFieldsFormatter{
			PrintLineNumber: true,
			DefaultFields:   logrus.Fields{"component": component},
		},
	)
}

// Format implements logrus.Formatter's Format. We allocate a new Fields
// map in order to not modify the caller's Entry, as that is not a thread
// safe operation.
func (f *DefaultFieldsFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	data := make(logrus.Fields, len(entry.Data)+len(f.DefaultFields))
	for k, v := range f.DefaultFields {
		data[k] = v
	}
	for k, v := range entry.Data {
		data[k] = v
	}
	return f.WrappedFormatter.Format(&logrus.Entry{
		Logger:  entry.Logger,
		Data:    data,
		Time:    entry.Time,
		Level:   entry.Level,
		Message: entry.Message,
		Caller:  entry.Caller,
	})
}

// CensoringFormatter represents a logrus formatter that
// can be used to censor sensitive information
type CensoringFormatter struct {
	delegate   logrus.Formatter
	getSecrets func() sets.String
}

func (f CensoringFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	raw, err := f.delegate.Format(entry)
	if err != nil {
		return raw, err
	}
	return f.censor(raw), nil
}

const censored = "CENSORED"

var (
	censoredBytes = []byte(censored)
	standardLog   = logrus.NewEntry(logrus.StandardLogger())
)

// Censor replaces sensitive parts of the content with a placeholder.
func (f CensoringFormatter) censor(content []byte) []byte {
	for _, secret := range f.getSecrets().List() {
		trimmedSecret := strings.TrimSpace(secret)
		if trimmedSecret != secret {
			standardLog.Warning("Secret is not trimmed")
			secret = trimmedSecret
		}
		if secret == "" {
			standardLog.Warning("Secret is an empty string, ignoring")
			continue
		}
		content = bytes.ReplaceAll(content, []byte(secret), censoredBytes)
	}
	return content
}

// NewCensoringFormatter generates a `CensoringFormatter` with
// a formatter as delegate and a set of strings to censor
func NewCensoringFormatter(f logrus.Formatter, getSecrets func() sets.String) CensoringFormatter {
	return CensoringFormatter{
		getSecrets: getSecrets,
		delegate:   f,
	}
}

// SpamCanFormatter is a logrus.Formatter that wraps another formatter
// and ensures that repeated entries are not re-logged
type SpamCanFormatter struct {
	mu               sync.Mutex
	observedHashes   map[string]struct{}
	repeatedEntries  map[string]repeatedEntry
	wrappedFormatter logrus.Formatter
	logger           *logrus.Logger
}

// NewSpamCanFormatter returns a new SpamCanFormatter wrapping logger and formatterToWrap
// It should be the top formatter in the wrapped chain of formatters
func NewSpamCanFormatter(logger *logrus.Logger, formatterToWrap logrus.Formatter) *SpamCanFormatter {
	return &SpamCanFormatter{
		wrappedFormatter: formatterToWrap,
		logger:           logger,
	}
}

type repeatedEntry struct {
	entry         *logrus.Entry
	firstObserved time.Time
	count         int
}

// Flush logs all repeated entries with a warning about having been repeated
// and clears them from memory
// This should be called periodically
func (s *SpamCanFormatter) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, repeatEntry := range s.repeatedEntries {
		s.logger.WithField("repeatedEntry", repeatEntry.entry).Warnf("entry repeated %d times since %v", repeatEntry.count, repeatEntry.firstObserved)
	}
	// separate loops because go will optimize this idiom
	for k := range s.observedHashes {
		delete(s.observedHashes, k)
	}
	for k := range s.repeatedEntries {
		delete(s.repeatedEntries, k)
	}
}

// Format implements logrus.Formatter
func (s *SpamCanFormatter) Format(entry *logrus.Entry) {
	hash, err := hashEntry(entry)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to hash log entry")
	}
	if err != nil || !s.observe(hash, entry) {
		s.wrappedFormatter.Format(entry)
	}
}

// haveSeen returns true if we have seen this entry hash since the last flush
func (s *SpamCanFormatter) haveSeen(hash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, seen := s.observedHashes[hash]
	return seen
}

// observe records an entry by hash and returns if we have previously recorded
// observing an entry the first time will return false, repeated observations
// of the same hash will return true and start counting the repeats
func (s *SpamCanFormatter) observe(hash string, entry *logrus.Entry) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, seen := s.observedHashes[hash]
	if !seen {
		s.observedHashes[hash] = struct{}{}
		return false
	}
	// we have seen it before, so record it as a repeated entry
	repeatEntry, alreadyRecorded := s.repeatedEntries[hash]
	if !alreadyRecorded {
		// create a new entry if this is the first time
		repeatEntry = repeatedEntry{
			entry:         entry,
			firstObserved: time.Now(),
			count:         1,
		}
	}
	// increment count and update
	repeatEntry.count++
	s.repeatedEntries[hash] = repeatEntry
	return true
}

func hashEntry(entry *logrus.Entry) (string, error) {
	data := make(logrus.Fields, len(entry.Data))
	for k, v := range entry.Data {
		data[k] = v
	}
	trimmedEntry := &logrus.Entry{
		Logger:  entry.Logger,
		Data:    data,
		Time:    time.Unix(0, 0), // we don't want timestamps to vary
		Level:   entry.Level,
		Message: entry.Message,
		Caller:  entry.Caller,
	}
	formatted, err := trimmedEntry.String()
	if err != nil {
		return "", err
	}
	// NOTE: md5 because the collision chance is not super high still
	// and we don't need to be cryptographically secure, just fast and
	// noting repeats
	hashed := md5.Sum([]byte(formatted))
	return string(hashed[:]), nil
}
