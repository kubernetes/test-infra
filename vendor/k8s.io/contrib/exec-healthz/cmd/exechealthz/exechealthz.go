/*
Copyright 2016 The Kubernetes Authors.

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

// A tiny web server that returns 200 on it's healthz endpoint if the command
// passed in via --cmd exits with 0. Returns 503 otherwise.
// Usage: exechealthz --port 8080 --period 2s --latency 30s --cmd 'nslookup localhost >/dev/null'
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"

	"k8s.io/contrib/exec-healthz/pkg/version"
	"k8s.io/kubernetes/pkg/util/clock"
	utilexec "k8s.io/kubernetes/pkg/util/exec"
)

// TODO:
// 1. Sigterm handler for docker stop
// 2. Meaningful default healthz
// 3. 404 for unknown endpoints

var (
	cmdDefault = "echo healthz"
	cmds       = flagStringArray("cmd", "Command to run in response to a GET on /healthz (or customized). If the given command exits with 0, /healthz will respond with a 200.")
	urlDefault = "/healthz"
	urls       = flagStringArray("url", "Path to serve on for cmd.")
	port       = flag.Int("port", 8080, "Port number to serve /healthz (or customized path).")
	period     = flag.Duration("period", 2*time.Second, "Period to run the given cmd in an async worker.")
	maxLatency = flag.Duration("latency", 30*time.Second, "If the async worker hasn't updated the probe command output in this long, return a 503.")
	quiet      = flag.Bool("quiet", false, "Run in quiet mode by only logging errors.")
	printVer   = flag.Bool("version", false, "Print the version and exit.")
	// probers are the async workers running the cmds, the output of which is used to service /healthz or others customized pathes.
	probers = make(map[string]*execWorker)
)

type arrayFlags []string

func flagStringArray(flagName, desc string) *arrayFlags {
	var newArrayFlags arrayFlags
	flag.Var(&newArrayFlags, flagName, desc)
	return &newArrayFlags
}

func flagCheckUrlCmd() {
	if len(*urls) == 0 {
		*urls = arrayFlags{urlDefault}
	}
	if len(*cmds) == 0 {
		*cmds = arrayFlags{cmdDefault}
	}
	if len(*cmds) != len(*urls) {
		log.Fatalf("failed to initialize flag, number of cmds and urls are different")
	}
}

func (i *arrayFlags) String() string {
	return fmt.Sprintf("%v", *i)
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

// execResult holds the result of the latest exec from the execWorker.
type execResult struct {
	output []byte
	err    error
	ts     time.Time
}

func (r execResult) String() string {
	errMsg := "None"
	if r.err != nil {
		errMsg = fmt.Sprintf("%v", r.err)
	}
	return fmt.Sprintf("Result of last exec: %v, at %v, error %v", string(r.output), r.ts, errMsg)
}

// execWorker provides an async interface to exec.
type execWorker struct {
	exec      utilexec.Interface
	clock     clock.Clock
	result    execResult
	mutex     sync.Mutex
	period    time.Duration
	probeCmd  string
	probePath string
	stopCh    chan struct{}
	readyCh   chan<- struct{} // For testing.
}

// getResults returns the results of the latest execWorker run.
// The caller should treat returned results as read-only.
func (h *execWorker) getResults() execResult {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return h.result
}

// logf logs the message, unless we run in quiet mode.
func logf(format string, args ...interface{}) {
	if !*quiet {
		log.Printf(format, args...)
	}
}

// start attemtps to run the probeCmd every `period` seconds.
// Meant to be called as a goroutine.
func (h *execWorker) start() {
	ticker := h.clock.Tick(h.period)
	h.readyCh <- struct{}{} // For testing.

	for {
		select {
		// If the command takes > period, the command runs continuously.
		case <-ticker:
			logf("Worker running %v to serve %v", h.probeCmd, h.probePath)
			output, err := h.exec.Command("sh", "-c", h.probeCmd).CombinedOutput()
			ts := h.clock.Now()
			func() {
				h.mutex.Lock()
				defer h.mutex.Unlock()
				h.result = execResult{output, err, ts}
			}()
		case <-h.stopCh:
			return
		}
	}
}

// newExecWorker is a constructor for execWorker.
func newExecWorker(probeCmd, probePath string, execPeriod time.Duration, exec utilexec.Interface, clock clock.Clock, readyCh chan<- struct{}) *execWorker {
	return &execWorker{
		// Initializing the result with a timestamp here allows us to
		// wait maxLatency for the worker goroutine to start, and for each
		// iteration of the worker to complete.
		exec:      exec,
		clock:     clock,
		result:    execResult{[]byte{}, nil, clock.Now()},
		period:    execPeriod,
		probeCmd:  probeCmd,
		probePath: probePath,
		stopCh:    make(chan struct{}),
		readyCh:   readyCh,
	}
}

func main() {
	flag.Parse()

	if *printVer {
		fmt.Printf("%s\n", version.VERSION)
		os.Exit(0)
	}

	flagCheckUrlCmd()
	links := []struct {
		link, desc string
	}{
		{"/healthz", "healthz probe. Returns \"ok\" if the command given through -cmd exits with 0."},
		{"/quit", "Cause this container to exit."},
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "<b> Kubernetes healthz sidecar container </b><br/><br/>")
		for _, v := range links {
			fmt.Fprintf(w, `<a href="%v">%v: %v</a><br/>`, v.link, v.link, v.desc)
		}
	})

	http.HandleFunc("/quit", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Shutdown requested via /quit by %v", r.RemoteAddr)
		os.Exit(0)
	})

	for i := range *urls {
		prober := newExecWorker((*cmds)[i], (*urls)[i], *period, utilexec.New(), clock.RealClock{}, make(chan struct{}, 1))
		probers[(*urls)[i]] = prober
		defer func() {
			close(prober.stopCh)
			close(prober.readyCh)
		}()
		go prober.start()

		http.HandleFunc((*urls)[i], healthzHandler)
	}

	log.Fatal(http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", *port), nil))
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	prober := probers[r.URL.Path]
	logf("Client ip %v requesting %v probe servicing cmd %v", r.RemoteAddr, prober.probePath, prober.probeCmd)
	result := prober.getResults()

	// return 503 if the last command exec returned a non-zero status, or the worker
	// hasn't run in maxLatency (including when the worker goroutine is cpu starved,
	// because the pod is probably unavailable too).
	if result.err != nil {
		msg := fmt.Sprintf("Healthz probe on %v error: %v", prober.probePath, result)
		log.Printf(msg)
		http.Error(w, msg, http.StatusServiceUnavailable)
	} else if prober.clock.Since(result.ts) > *maxLatency {
		msg := fmt.Sprintf("Latest result too old to be useful: %v.", result)
		log.Printf(msg)
		http.Error(w, msg, http.StatusServiceUnavailable)
	} else {
		fmt.Fprintf(w, "ok")
	}
}
