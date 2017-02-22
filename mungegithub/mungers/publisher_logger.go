/*
Copyright 2017 The Kubernetes Authors.

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

// Changing glog output directory via --log_dir doesn't work, because the flag
// is parsed after the first invocation of glog, so the log file ends up in the
// temporary directory. Hence, we manually duplicates glog ouptut.

package mungers

import (
	"bytes"
	"fmt"
	"time"

	"github.com/golang/glog"
)

type plog struct {
	buf *bytes.Buffer
}

func NewPublisherLog(buf *bytes.Buffer) *plog {
	return &plog{buf}
}

func (p *plog) write(format string, args ...interface{}) {
	p.buf.WriteString("[" + time.Now().Format(time.RFC822) + "]: ")
	p.buf.WriteString(fmt.Sprintf(format, args...))
	p.buf.WriteString("\n")
}

func (p *plog) Errorf(format string, args ...interface{}) {
	glog.Errorf(format, args...)
	p.write(format, args...)
}

func (p *plog) Infof(format string, args ...interface{}) {
	glog.Infof(format, args...)
	p.write(format, args...)
}

func (p *plog) Fatalf(format string, args ...interface{}) {
	glog.Fatalf(format, args...)
	p.write(format, args...)
}

func (p *plog) ReadLog() string {
	return p.buf.String()
}
