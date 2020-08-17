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
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/version"
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
func ComponentInit() {
	Init(
		&DefaultFieldsFormatter{
			PrintLineNumber: true,
			DefaultFields:   logrus.Fields{"component": version.Name},
		},
	)
}

// Format implements logrus.Formatter's Format. We allocate a new Fields
// map in order to not modify the caller's Entry, as that is not a thread
// safe operation.
func (f *DefaultFieldsFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	data := make(logrus.Fields, len(entry.Data)+len(f.DefaultFields)+1)
	// GCP's log collection expects a "severity" field instead of "level"
	data["severity"] = entry.Level
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
