/*
Copyright 2020 The Kubernetes Authors.

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

package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
)

type fakeListenAndServer struct {
	ctx    context.Context
	server *httptest.Server
}

func (fls *fakeListenAndServer) ListenAndServe() error {
	defer fls.server.Close()
	// Already listening and serving
	<-fls.ctx.Done()
	return http.ErrServerClosed
}

func (fls *fakeListenAndServer) Shutdown(ctx context.Context) error {
	return fls.server.Config.Shutdown(ctx)
}

func (fls *fakeListenAndServer) CreateServer(handler http.Handler) interrupts.ListenAndServer {
	fls.server = httptest.NewServer(handler)
	return fls
}

func TestExposeMetrics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fls := fakeListenAndServer{ctx: ctx}

	ExposeMetricsWithRegistry("my-component", config.PushGateway{}, flagutil.DefaultMetricsPort, nil, fls.CreateServer)
	resp, err := http.Get(fls.server.URL + "/metrics")
	if err != nil {
		t.Fatalf("failed getting metrics: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("resonse status was not 200 but %d", resp.StatusCode)
	}
}
