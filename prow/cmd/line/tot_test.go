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

package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func expectEqual(t *testing.T, msg string, have interface{}, want interface{}) {
	if !reflect.DeepEqual(have, want) {
		t.Errorf("bad %s: got %v, wanted %v",
			msg, have, want)
	}
}

type fakeServer struct {
	errorCount int
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.errorCount > 0 {
		f.errorCount--
		http.Error(w, "not yet ready!", http.StatusInternalServerError)
		return
	}
	if r.URL.Path == "/vend/test" {
		fmt.Fprintf(w, "12345")
	} else {
		fmt.Fprintf(w, "56789")
	}
}

func TestTotVend(t *testing.T) {
	fake := &fakeServer{errorCount: 0}
	serv := httptest.NewServer(fake)
	defer serv.Close()

	expectEqual(t, "basic vending", getBuildID(serv.URL, "test"), "12345")
	expectEqual(t, "basic vending", getBuildID(serv.URL, "other"), "56789")

	retryDelay = 10
	fake.errorCount = 3
	expectEqual(t, "basic vending", getBuildID(serv.URL, "test"), "12345")
	expectEqual(t, "errors cleared", fake.errorCount, 0)

	rand.Seed(1)
	fake.errorCount = 60
	expectEqual(t, "vending never succeeds", getBuildID(serv.URL, "doomed"), "5577006791947779410")
}
