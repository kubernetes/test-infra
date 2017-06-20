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

package kube

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func getClient(url string) *Client {
	return &Client{
		baseURL:   url,
		client:    &http.Client{},
		token:     "abcd",
		namespace: "ns",
	}
}

func TestNamespace(t *testing.T) {
	c1 := &Client{
		baseURL:   "a",
		namespace: "ns1",
	}
	c2 := c1.Namespace("ns2")
	if c1 == c2 {
		t.Error("Namespace modified in place.")
	}
	if c2.baseURL != c1.baseURL {
		t.Error("Didn't copy over struct members.")
	}
	if c2.namespace != "ns2" {
		t.Errorf("Got wrong namespace. Got %s, expected ns2", c2.namespace)
	}
}

func TestListPods(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/namespaces/ns/pods" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"items": [{}, {}]}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	ps, err := c.ListPods(nil)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if len(ps) != 2 {
		t.Error("Expected two pods.")
	}
}

func TestDeletePod(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/namespaces/ns/pods/po" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	err := c.DeletePod("po")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestGetPod(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/namespaces/ns/pods/po" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"metadata": {"name": "abcd"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	po, err := c.GetPod("po")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if po.Metadata.Name != "abcd" {
		t.Errorf("Wrong name: %s", po.Metadata.Name)
	}
}

func TestCreatePod(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/namespaces/ns/pods" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"metadata": {"name": "abcd"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	po, err := c.CreatePod(Pod{})
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if po.Metadata.Name != "abcd" {
		t.Errorf("Wrong name: %s", po.Metadata.Name)
	}
}

// TestNewClient messes around with certs and keys and such to just make sure
// that our cert handling is done properly. We create root and client keys,
// then server and client certificates, then ensure that the client can talk
// to the server.
// See https://ericchiang.github.io/post/go-tls/ for implementation details.
func TestNewClient(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	rootKey, err := rsa.GenerateKey(r, 2048)
	if err != nil {
		t.Fatalf("Generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(42),
		SignatureAlgorithm:    x509.SHA256WithRSA,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:        true,
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(r, tmpl, tmpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("Creating cert: %v", err)
	}
	rootCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Parsing cert: %v", err)
	}
	rootCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	rootKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey),
	})
	rootTLSCert, err := tls.X509KeyPair(rootCertPEM, rootKeyPEM)
	if err != nil {
		t.Fatalf("Creating KeyPair: %v", err)
	}

	clientKey, err := rsa.GenerateKey(r, 2048)
	if err != nil {
		t.Fatalf("Creating key: %v", err)
	}

	clientCertTmpl := &x509.Certificate{
		BasicConstraintsValid: true,
		SerialNumber:          big.NewInt(43),
		SignatureAlgorithm:    x509.SHA256WithRSA,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientCertDER, err := x509.CreateCertificate(r, clientCertTmpl, rootCert, &clientKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("Creating cert: %v", err)
	}
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey),
	})

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(rootCertPEM)

	clus := &Cluster{
		ClientCertificate:    base64.StdEncoding.EncodeToString(clientCertPEM),
		ClientKey:            base64.StdEncoding.EncodeToString(clientKeyPEM),
		ClusterCACertificate: base64.StdEncoding.EncodeToString(rootCertPEM),
	}
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) }))
	s.TLS = &tls.Config{
		Certificates: []tls.Certificate{rootTLSCert},
		ClientCAs:    certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	s.StartTLS()
	defer s.Close()
	clus.Endpoint = s.URL
	cl, err := NewClient(clus, "default")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	if _, err := cl.GetPod("p"); err != nil {
		t.Fatalf("Failed to talk to server: %v", err)
	}
}
