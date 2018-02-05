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
	"bufio"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"k8s.io/api/core/v1"
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

func TestSetHiddenReposProviderGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		switch r.URL.Path {
		case "/apis/prow.k8s.io/v1/namespaces/ns/prowjobs/pja":
			fmt.Fprint(w, `{"spec": {"job": "a", "refs": {"org": "org", "repo": "repo"}}}`)
		case "/apis/prow.k8s.io/v1/namespaces/ns/prowjobs/pjb":
			fmt.Fprint(w, `{"spec": {"job": "b", "refs": {"org": "hidden-org", "repo": "repo"}}}`)
		default:
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	c.SetHiddenReposProvider(func() []string { return []string{"hidden-org"} }, false)
	pj, err := c.GetProwJob("pja")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if got, expected := pj.Spec.Job, "a"; got != expected {
		t.Errorf("Expected returned prowjob to be job %q, but got %q.", expected, got)
	}

	pj, err = c.GetProwJob("pjb")
	if err == nil {
		t.Fatal("Expected error getting hidden prowjob, but did not receive an error.")
	}
}

func TestHiddenReposProviderGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		switch r.URL.Path {
		case "/apis/prow.k8s.io/v1/namespaces/ns/prowjobs/pja":
			fmt.Fprint(w, `{"spec": {"job": "a", "refs": {"org": "org", "repo": "repo"}}}`)
		case "/apis/prow.k8s.io/v1/namespaces/ns/prowjobs/pjb":
			fmt.Fprint(w, `{"spec": {"job": "b", "refs": {"org": "hidden-org", "repo": "repo"}}}`)
		default:
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	c.SetHiddenReposProvider(func() []string { return []string{"hidden-org"} }, true)
	pj, err := c.GetProwJob("pjb")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if got, expected := pj.Spec.Job, "b"; got != expected {
		t.Errorf("Expected returned prowjob to be job %q, but got %q.", expected, got)
	}
}

func TestSetHiddenReposProviderList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/apis/prow.k8s.io/v1/namespaces/ns/prowjobs" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"items": [{"spec": {"job": "a", "refs": {"org": "org", "repo": "hidden-repo"}}}, {"spec": {"job": "b", "refs": {"org": "org", "repo": "repo"}}}]}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	c.SetHiddenReposProvider(func() []string { return []string{"org/hidden-repo"} }, false)
	pjs, err := c.ListProwJobs(EmptySelector)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if len(pjs) != 1 {
		t.Fatalf("Expected one prowjobs, but got %v.", pjs)
	}
	if got, expected := pjs[0].Spec.Job, "b"; got != expected {
		t.Errorf("Expected returned prowjob to be job %q, but got %q.", expected, got)
	}
}

func TestHiddenReposProviderList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/apis/prow.k8s.io/v1/namespaces/ns/prowjobs" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"items": [{"spec": {"job": "a", "refs": {"org": "org", "repo": "hidden-repo"}}}, {"spec": {"job": "b", "refs": {"org": "org", "repo": "repo"}}}]}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	c.SetHiddenReposProvider(func() []string { return []string{"org/hidden-repo"} }, true)
	pjs, err := c.ListProwJobs(EmptySelector)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if len(pjs) != 1 {
		t.Fatalf("Expected one prowjobs, but got %v.", pjs)
	}
	if got, expected := pjs[0].Spec.Job, "a"; got != expected {
		t.Errorf("Expected returned prowjob to be job %q, but got %q.", expected, got)
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
	ps, err := c.ListPods(EmptySelector)
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
	if po.ObjectMeta.Name != "abcd" {
		t.Errorf("Wrong name: %s", po.ObjectMeta.Name)
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
	po, err := c.CreatePod(v1.Pod{})
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if po.ObjectMeta.Name != "abcd" {
		t.Errorf("Wrong name: %s", po.ObjectMeta.Name)
	}
}

func TestCreateConfigMap(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/namespaces/ns/configmaps" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"metadata": {"name": "abcd"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if _, err := c.CreateConfigMap(ConfigMap{}); err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestReplaceConfigMap(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/namespaces/ns/configmaps/config" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"metadata": {"name": "abcd"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	if _, err := c.ReplaceConfigMap("config", ConfigMap{}); err != nil {
		t.Errorf("Didn't expect error: %v", err)
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

type tempConfig struct {
	file   *os.File
	writer *bufio.Writer
}

func newTempConfig() (*tempConfig, error) {
	tempfile, err := ioutil.TempFile(os.TempDir(), "prow_kube_client_test")
	if err != nil {
		return nil, err
	}
	return &tempConfig{file: tempfile, writer: bufio.NewWriter(tempfile)}, nil
}

func (t *tempConfig) SetContent(content string) error {
	// Clear file and reset writing offset
	t.file.Truncate(0)
	t.file.Seek(0, os.SEEK_SET)
	t.writer.Reset(t.file)
	if _, err := t.writer.WriteString(content); err != nil {
		return err
	}
	if err := t.writer.Flush(); err != nil {
		return err
	}
	return nil
}

func (t *tempConfig) Clean() {
	t.file.Close()
	os.Remove(t.file.Name())
}

func TestClientMapFromFile(t *testing.T) {
	newClient = func(c *Cluster, namespace string) (*Client, error) {
		return &Client{baseURL: c.Endpoint}, nil
	}
	defer func() { newClient = NewClient }()

	temp, err := newTempConfig()
	if err != nil {
		t.Fatalf("Failed to create temp file for test: %v", err)
	}
	defer temp.Clean()

	testCases := []struct {
		name           string
		configContents string
		expectedMap    map[string]*Client
	}{
		{
			name: "single cluster config",
			configContents: `endpoint: "cluster1"
clientKey: "key1"
`,
			expectedMap: map[string]*Client{
				DefaultClusterAlias: {baseURL: "cluster1"},
			},
		},
		{
			name: "multi cluster config",
			configContents: `"default":
  endpoint: "cluster1"
  clientKey: "key1"
"trusted":
  endpoint: "cluster2"
  clientKey: "key2"
`,
			expectedMap: map[string]*Client{
				DefaultClusterAlias: {baseURL: "cluster1"},
				"trusted":           {baseURL: "cluster2"},
			},
		},
		{
			name: "multi cluster config missing 'default' key",
			configContents: `"untrusted":
  endpoint: "cluster1"
  clientKey: "key1"
"trusted":
  endpoint: "cluster2"
  clientKey: "key2"
`,
			expectedMap: nil,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test scenario %q...", tc.name)
		if err := temp.SetContent(tc.configContents); err != nil {
			t.Fatalf("Error setting temp file contents: %v", err)
		}
		m, err := ClientMapFromFile(temp.file.Name(), "ns")
		if err != nil && tc.expectedMap != nil {
			t.Fatalf("Unexpected error loading config: %v.", err)
		} else if err == nil && tc.expectedMap == nil {
			t.Fatal("Expected an error loading the config, but did not receive one!")
		}
		if expect, got := tc.expectedMap, m; !reflect.DeepEqual(expect, got) {
			t.Errorf("Expected cluster config to produce map %v, but got %v.", expect, got)
		}
	}
}
