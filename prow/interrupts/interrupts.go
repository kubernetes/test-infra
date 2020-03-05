/*
Copyright 2019 The Kubernetes Authors.

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

// Package interrupts exposes helpers for graceful handling of interrupt signals
package interrupts

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// only one instance of the manager ever exists
var single *manager

func init() {
	m := sync.Mutex{}
	single = &manager{
		c:  sync.NewCond(&m),
		wg: sync.WaitGroup{},
	}
	go handleInterrupt()
}

type manager struct {
	// only one signal handler should be installed, so we use a cond to
	// broadcast to workers that an interrupt has occurred
	c *sync.Cond
	// we record whether we've broadcast in the past
	seenSignal bool
	// we want to ensure that all registered servers and workers get a
	// change to gracefully shut down
	wg sync.WaitGroup
}

// handleInterrupt turns an interrupt into a broadcast for our condition.
// This must be called _first_ before any work is registered with the
// manager, or there will be a deadlock.
func handleInterrupt() {
	signalsLock.Lock()
	sigChan := signals()
	signalsLock.Unlock()
	s := <-sigChan
	logrus.WithField("signal", s).Info("Received signal.")
	single.c.L.Lock()
	single.seenSignal = true
	single.c.Broadcast()
	single.c.L.Unlock()
}

// test initialization will set the signals channel in another goroutine
// so we need to synchronize that in order to not trigger the race detector
// even though we know that init() calls will be serial and the test init()
// will fire first
var signalsLock = sync.Mutex{}

// signals allows for injection of mock signals in testing
var signals = func() <-chan os.Signal {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	return sig
}

// wait executes the cancel when an interrupt is seen or if one has already
// been handled
func wait(cancel func()) {
	single.c.L.Lock()
	if !single.seenSignal {
		single.c.Wait()
	}
	single.c.L.Unlock()
	cancel()
}

var gracePeriod = 1 * time.Minute

// WaitForGracefulShutdown waits until all registered servers and workers
// have had time to gracefully shut down, or times out. This function is
// blocking.
func WaitForGracefulShutdown() {
	wait(func() {
		logrus.Info("Interrupt received.")
	})
	finished := make(chan struct{})
	go func() {
		single.wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
		logrus.Info("All workers gracefully terminated, exiting.")
	case <-time.After(gracePeriod):
		logrus.Warn("Timed out waiting for workers to gracefully terminate, exiting.")
	}
}

// Context returns a context that stays is cancelled when an interrupt hits.
// Using this context is a weak guarantee that your work will finish before
// process exit as callers cannot signal that they are finished. Prefer to use
// Run().
func Context() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	single.wg.Add(1)
	go wait(func() {
		cancel()
		single.wg.Done()
	})

	return ctx
}

// Run will do work until an interrupt is received, then signal the
// worker. This function is not blocking. Callers are expected to exit
// only after WaitForGracefulShutdown returns to ensure all workers have
// had time to shut down. This is preferable to getting the raw Context
// as we can ensure that the work is finished before releasing our share
// of the wait group on shutdown.
func Run(work func(ctx context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())
	single.wg.Add(1)
	go func() {
		defer single.wg.Done()
		work(ctx)
	}()

	go wait(cancel)
}

// ListenAndServer is typically an http.Server
type ListenAndServer interface {
	Shutdownable
	ListenAndServe() error
}

// ListenAndServe runs the HTTP server and handles shutting it down
// gracefully on interrupts. This function is not blocking. Callers
// are expected to exit only after WaitForGracefulShutdown returns to
// ensure all servers have had time to shut down.
func ListenAndServe(server ListenAndServer, gracePeriod time.Duration) {
	single.wg.Add(1)
	go func() {
		defer single.wg.Done()
		logrus.WithError(server.ListenAndServe()).Info("Server exited.")
	}()

	go wait(shutdown(server, gracePeriod))
}

// ListenAndServeTLS runs the HTTP server and handles shutting it down
// gracefully on interrupts. This function is not blocking. Callers
// are expected to exit only after WaitForGracefulShutdown returns to
// ensure all servers have had time to shut down.
func ListenAndServeTLS(server *http.Server, certFile, keyFile string, gracePeriod time.Duration) {
	single.wg.Add(1)
	go func() {
		defer single.wg.Done()
		logrus.WithError(server.ListenAndServeTLS(certFile, keyFile)).Info("Server exited.")
	}()

	go wait(shutdown(server, gracePeriod))
}

// Shutdownable is typically an http.Server
type Shutdownable interface {
	Shutdown(context.Context) error
}

// shutdown will shut down the server
func shutdown(server Shutdownable, gracePeriod time.Duration) func() {
	return func() {
		logrus.Info("Server shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), gracePeriod)
		if err := server.Shutdown(ctx); err != nil {
			logrus.WithError(err).Info("Error shutting down server...")
		}
		cancel()
	}
}

// Tick will do work on a dynamically determined interval until an
// interrupt is received. This function is not blocking. Callers are
// expected to exit only after WaitForGracefulShutdown returns to
// ensure all workers have had time to shut down.
func Tick(work func(), interval func() time.Duration) {
	before := time.Time{} // we want to do work right away
	sig := make(chan int, 1)
	single.wg.Add(1)
	go func() {
		defer single.wg.Done()
		for {
			nextInterval := interval()
			nextTick := before.Add(nextInterval)
			sleep := time.Until(nextTick)
			logrus.WithFields(logrus.Fields{
				"before":   before,
				"interval": nextInterval,
				"sleep":    sleep,
			}).Debug("Resolved next tick interval.")
			select {
			case <-time.After(sleep):
				before = time.Now()
				work()
			case <-sig:
				logrus.Info("Worker shutting down...")
				return
			}
		}
	}()

	go wait(func() {
		sig <- 1
	})
}

// TickLiteral runs Tick with an unchanging interval.
func TickLiteral(work func(), interval time.Duration) {
	Tick(work, func() time.Duration {
		return interval
	})
}

// OnInterrupt ensures that work is done when an interrupt is fired
// and that we wait for the work to be finished before we consider
// the process cleaned up. This function is not blocking.
func OnInterrupt(work func()) {
	single.wg.Add(1)
	go wait(func() {
		work()
		single.wg.Done()
	})
}
