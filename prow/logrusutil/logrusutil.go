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
	"github.com/sirupsen/logrus"
)

// DefaultFieldsFormatter wraps another logrus.Formatter, injecting
// DefaultFields into each Format() call, existing fields are preserved
// if they have the same key
type DefaultFieldsFormatter struct {
	WrappedFormatter logrus.Formatter
	DefaultFields    logrus.Fields
}

// NewDefaultFieldsFormatter returns a DefaultFieldsFormatter,
// if wrappedFormatter is nil &logrus.JSONFormatter{} will be used instead
func NewDefaultFieldsFormatter(
	wrappedFormatter logrus.Formatter, defaultFields logrus.Fields,
) *DefaultFieldsFormatter {
	res := &DefaultFieldsFormatter{
		WrappedFormatter: wrappedFormatter,
		DefaultFields:    defaultFields,
	}
	if res.WrappedFormatter == nil {
		res.WrappedFormatter = &logrus.JSONFormatter{}
	}
	return res
}

// Format implements logrus.Formatter's Format. We allocate a new Fields
// map in order to not modify the caller's Entry, as that is not a thread
// safe operation.
func (d *DefaultFieldsFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	data := make(logrus.Fields, len(entry.Data)+len(d.DefaultFields))
	for k, v := range d.DefaultFields {
		data[k] = v
	}
	for k, v := range entry.Data {
		data[k] = v
	}
	return d.WrappedFormatter.Format(&logrus.Entry{
		Logger:  entry.Logger,
		Data:    data,
		Time:    entry.Time,
		Level:   entry.Level,
		Message: entry.Message,
	})
}
