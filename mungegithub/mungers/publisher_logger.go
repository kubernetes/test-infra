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
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/shurcooL/go/indentwriter"
)

type plog struct {
	buf *bytes.Buffer
}

func NewPublisherLog(buf *bytes.Buffer) *plog {
	return &plog{buf}
}

func (p *plog) write(s string) {
	p.buf.WriteString("[" + time.Now().Format(time.RFC822) + "]: ")
	p.buf.WriteString(s)
	p.buf.WriteString("\n")
}

func (p *plog) Errorf(format string, args ...interface{}) {
	s := prefixFollowingLines("    ", fmt.Sprintf(format, args...))
	glog.ErrorDepth(1, s)
	p.write(s)
}

func (p *plog) Infof(format string, args ...interface{}) {
	s := prefixFollowingLines("    ", fmt.Sprintf(format, args...))
	glog.InfoDepth(1, s)
	p.write(s)
}

func (p *plog) Fatalf(format string, args ...interface{}) {
	s := prefixFollowingLines("    ", fmt.Sprintf(format, args...))
	glog.FatalDepth(1, s)
	p.write(s)
}

func (p *plog) Run(c *exec.Cmd) error {
	p.Infof("%s", cmdStr(*c))

	errBuf := &bytes.Buffer{}

	c.Stdout = indentwriter.New(&muxWriter{p.buf, os.Stdout}, 1)
	c.Stderr = indentwriter.New(errBuf, 1)

	err := c.Start()
	if err != nil {
		p.Errorf("failed to start %q: %v", c.Path, err)
		return err
	}
	err = c.Wait()
	if err != nil {
		p.Errorf("%s\n%s", err.Error(), errBuf.String())
	}
	return err
}

func (p *plog) ReadLog() string {
	return p.buf.String()
}

func (p *plog) Flush() {
	glog.Flush()
}

func prefixFollowingLines(p, s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		if i != 0 && len(lines[i]) > 0 {
			lines[i] = p + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

type cmdStr exec.Cmd

func (cs cmdStr) String() string {
	args := make([]string, len(cs.Args))
	for i, s := range cs.Args {
		if strings.IndexRune(s, ' ') != -1 {
			args[i] = fmt.Sprintf("%q", s)
		} else {
			args[i] = s
		}
	}
	return strings.Join(args, " ")
}

type muxWriter struct {
	A, B io.Writer
}

func (w *muxWriter) Write(b []byte) (int, error) {
	if n, err := w.A.Write(b); err != nil {
		return n, err
	}
	return w.B.Write(b)
}
