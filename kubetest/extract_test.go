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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestGetKube(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		success bool
	}{
		{
			name:    "can succeed",
			script:  "true",
			success: true,
		},
		{
			name:    "can fail",
			script:  "exit 1",
			success: false,
		},
		{
			name:    "can successfully retry",
			script:  "([[ -e ran ]] && true) || (touch ran && exit 1)",
			success: true,
		},
	}

	if !terminate.Stop() {
		<-terminate.C
	}
	if !interrupt.Stop() {
		<-interrupt.C
	}

	oldSleep := sleep
	defer func() { sleep = oldSleep }()
	sleep = func(d time.Duration) {}

	if o, err := os.Getwd(); err != nil {
		t.Fatal(err)
	} else {
		defer os.Chdir(o)
	}
	if d, err := ioutil.TempDir("", "extract"); err != nil {
		t.Fatal(err)
	} else if err := os.Chdir(d); err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		bytes := []byte(fmt.Sprintf("#!/bin/bash\necho hello\n%s\nmkdir -p ./kubernetes/cluster && touch ./kubernetes/cluster/get-kube-binaries.sh", tc.script))
		if err := ioutil.WriteFile("./get-kube.sh", bytes, 0700); err != nil {
			t.Fatal(err)
		}
		err := getKube("url", "version")
		if tc.success && err != nil {
			t.Errorf("%s did not succeed: %s", tc.name, err)
		}
		if !tc.success && err == nil {
			t.Errorf("%s unexpectedly succeeded", tc.name)
		}
	}
}
