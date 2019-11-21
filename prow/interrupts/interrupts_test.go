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

package interrupts

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

// interrupt allows for tests to trigger an interrupt as needed
var interrupt = make(chan os.Signal, 1)

// this init will be executed before that in the code package,
// so we can inject our implementation of the interrupt channel
func init() {
	signalsLock.Lock()
	gracePeriod = time.Second
	signals = func() <-chan os.Signal {
		return interrupt
	}
	signalsLock.Unlock()
}

// instead of building a mechanism to reset/re-initialize the interrupt
// manager which would only be used in testing, we write an integration
// test that only fires the mock interrupt once
func TestInterrupts(t *testing.T) {
	// we need to lock around values used to test otherwise the test
	// goroutine will race with the workers
	lock := sync.Mutex{}

	ctx := Context()
	var ctxDone bool
	go func() {
		<-ctx.Done()

		lock.Lock()
		ctxDone = true
		lock.Unlock()
	}()

	var workDone bool
	var workCancelled bool
	work := func(ctx context.Context) {
		lock.Lock()
		workDone = true
		lock.Unlock()

		<-ctx.Done()

		lock.Lock()
		workCancelled = true
		lock.Unlock()
	}
	Run(work)

	// we cannot use httptest mocks for the tests here as they expect
	// to be started by the httptest package itself, not by a downstream
	// caller like the interrupts library
	var serverCalled bool
	var serverCancelled bool
	listener, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		t.Fatalf("could not listen on random port: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("could close listener: %v", err)
	}
	server := &http.Server{Addr: listener.Addr().String(), Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		lock.Lock()
		serverCalled = true
		lock.Unlock()
	})}
	server.RegisterOnShutdown(func() {
		lock.Lock()
		serverCancelled = true
		lock.Unlock()
	})
	ListenAndServe(server, time.Second)
	// wait for the server to start
	time.Sleep(100 * time.Millisecond)
	if _, err := http.Get("http://" + listener.Addr().String()); err != nil {
		t.Errorf("could not reach server registered with ListenAndServe(): %v", err)
	}

	var tlsServerCalled bool
	var tlsServerCancelled bool
	tlsListener, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		t.Fatalf("could not listen on random port: %v", err)
	}
	if err := tlsListener.Close(); err != nil {
		t.Fatalf("could close listener: %v", err)
	}
	tlsServer := &http.Server{Addr: tlsListener.Addr().String(), Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		lock.Lock()
		tlsServerCalled = true
		lock.Unlock()
	})}
	tlsServer.RegisterOnShutdown(func() {
		lock.Lock()
		tlsServerCancelled = true
		lock.Unlock()
	})
	cert, key, err := generateCerts("127.0.0.1")
	if err != nil {
		t.Fatalf("could not generate cert and key for TLS server: %v", err)
	}
	ListenAndServeTLS(tlsServer, cert, key, time.Second)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	// wait for the server to start
	time.Sleep(100 * time.Millisecond)
	if _, err := client.Get("https://" + tlsListener.Addr().String()); err != nil {
		t.Errorf("could not reach server registered with ListenAndServeTLS(): %v", err)
	}

	var intervalCalls int
	interval := func() time.Duration {
		lock.Lock()
		intervalCalls++
		lock.Unlock()
		if intervalCalls > 2 {
			return 10 * time.Hour
		}
		return 1 * time.Nanosecond
	}
	var tickCalls int
	tick := func() {
		lock.Lock()
		tickCalls++
		lock.Unlock()
	}
	Tick(tick, interval)
	// writing a test that functions correctly here without being susceptible
	// to timing flakes is challenging. Using time.Sleep like this does have
	// that downside, but the sleep time is many orders of magnitude higher
	// than the tick intervals and the amount of time taken to execute the
	// test as well, so it is going to be exceedingly rare that scheduling of
	// the test process will cause a flake here from timing. The test cannot
	// use synchronized approaches to waiting here as we do not know how long
	// we must wait. The test must have enough time to ask for the interval
	// as many times as we expect it to, but if we only wait for that we fail
	// to catch the cases where the interval is requested too many times.
	time.Sleep(100 * time.Millisecond)

	var onInterruptCalled bool
	OnInterrupt(func() {
		lock.Lock()
		onInterruptCalled = true
		lock.Unlock()
	})

	done := sync.WaitGroup{}
	done.Add(1)
	go func() {
		WaitForGracefulShutdown()
		done.Done()
	}()

	if onInterruptCalled {
		t.Error("work registered with OnInterrupt() was executed before interrupt")
	}

	// trigger the interrupt
	interrupt <- syscall.Signal(1)
	// wait for graceful shutdown to occur
	done.Wait()

	lock.Lock()
	if !ctxDone {
		t.Error("context from Context() was not cancelled on interrupt")
	}
	if !workDone {
		t.Error("work registered with Run() was not executed")
	}
	if !workCancelled {
		t.Error("work registered with Run() was not cancelled on interrupt")
	}
	if !serverCalled {
		t.Error("server registered with ListenAndServe() was not serving")
	}
	if !serverCancelled {
		t.Error("server registered with ListenAndServe() was not cancelled on interrupt")
	}
	if !tlsServerCalled {
		t.Error("server registered with ListenAndServeTLS() was not serving")
	}
	if !tlsServerCancelled {
		t.Error("server registered with ListenAndServeTLS() was not cancelled on interrupt")
	}
	if tickCalls != 2 {
		t.Errorf("work registered with Tick() was called %d times, not %d; interval was requested %d times", tickCalls, 2, intervalCalls)
	}
	if !onInterruptCalled {
		t.Error("work registered with OnInterrupt() was not executed on interrupt")
	}
	lock.Unlock()
}

func generateCerts(url string) (string, string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate serial number: %s", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(1 * time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		IPAddresses: []net.IP{net.ParseIP(url)},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate: %s", err)
	}

	certOut, err := ioutil.TempFile("", "cert.pem")
	if err != nil {
		return "", "", fmt.Errorf("failed to open cert.pem for writing: %s", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return "", "", fmt.Errorf("failed to write data to cert.pem: %s", err)
	}
	if err := certOut.Close(); err != nil {
		return "", "", fmt.Errorf("error closing cert.pem: %s", err)
	}

	keyOut, err := ioutil.TempFile("", "key.pem")
	if err != nil {
		return "", "", fmt.Errorf("failed to open key.pem for writing: %v", err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", fmt.Errorf("unable to marshal private key: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return "", "", fmt.Errorf("failed to write data to key.pem: %s", err)
	}
	if err := keyOut.Close(); err != nil {
		return "", "", fmt.Errorf("error closing key.pem: %s", err)
	}
	if err := os.Chmod(keyOut.Name(), 0600); err != nil {
		return "", "", fmt.Errorf("could not change permissions on key.pem: %v", err)
	}
	return certOut.Name(), keyOut.Name(), nil
}
