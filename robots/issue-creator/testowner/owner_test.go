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

package testowner

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"A":                                    "a",
		"Perf [Performance]":                   "perf",
		"[k8s.io] test [performance] stuff":    "test stuff",
		"[k8s.io] blah {Kubernetes e2e suite}": "blah",
	}
	for input, output := range tests {
		result := normalize(input)
		if result != output {
			t.Errorf("normalize(%s) != %s (got %s)", input, output, result)
		}
	}
}

func TestOwnerList(t *testing.T) {
	list := NewOwnerList(map[string]*OwnerInfo{"Perf [performance]": {
		User: "me",
		SIG:  "group",
	}})
	owner := list.TestOwner("perf [flaky]")
	if owner != "me" {
		t.Error("Unexpected return value ", owner)
	}
	sig := list.TestSIG("perf [flaky]")
	if sig != "group" {
		t.Error("Unexpected sig: ", sig)
	}
	owner = list.TestOwner("Unknown test")
	if owner != "" {
		t.Error("Unexpected return value ", owner)
	}
	sig = list.TestSIG("Unknown test")
	if sig != "" {
		t.Error("Unexpected sig: ", sig)
	}
}

func TestOwnerGlob(t *testing.T) {
	list := NewOwnerList(map[string]*OwnerInfo{"blah * [performance] test *": {
		User: "me",
		SIG:  "group",
	}})
	owner := list.TestOwner("blah 200 test foo")
	if owner != "me" {
		t.Error("Unexpected return value ", owner)
	}
	sig := list.TestSIG("blah 200 test foo")
	if sig != "group" {
		t.Error("Unexpected sig: ", sig)
	}
	owner = list.TestOwner("Unknown test")
	if owner != "" {
		t.Error("Unexpected return value ", owner)
	}
	sig = list.TestSIG("Unknown test")
	if sig != "" {
		t.Error("Unexpected sig: ", sig)
	}
}

func TestOwnerListFromCsv(t *testing.T) {
	r := bytes.NewReader([]byte(",,,header nonsense,\n" +
		",owner,suggested owner,name,sig\n" +
		",foo,other,Test name,Node\n" +
		", bar,foo,other test, Windows\n"))
	list, err := NewOwnerListFromCsv(r)
	if err != nil {
		t.Error(err)
	}
	if owner := list.TestOwner("test name"); owner != "foo" {
		t.Error("unexpected return value ", owner)
	}
	if sig := list.TestSIG("test name"); sig != "Node" {
		t.Error("unexpected sig value ", sig)
	}
	if owner := list.TestOwner("other test"); owner != "bar" {
		t.Error("unexpected return value ", owner)
	}
	if sig := list.TestSIG("other test"); sig != "Windows" {
		t.Error("unexpected sig value ", sig)
	}
}

func TestReloadingOwnerList(t *testing.T) {
	cases := []struct {
		name   string
		csv    string
		lookup string
		owner  string
		sig    string
		err    bool
	}{
		{
			name:   "owner and sig",
			csv:    "owner,name,sig\nfoo,flake,Scheduling\n",
			lookup: "flake",
			owner:  "foo",
			sig:    "Scheduling",
		},
		{
			name:   "missing sig returns badCsv",
			csv:    "owner,name,sig\nfoo,flake\n",
			lookup: "flake",
			err:    true,
		},
	}
	tempfile, err := ioutil.TempFile(os.TempDir(), "ownertest")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(tempfile.Name())
	defer tempfile.Close()
	writer := bufio.NewWriter(tempfile)

	for _, tc := range cases {
		// Assuming millisecond resolution on our FS, this sleep
		// ensures the mtime will change with the next write.
		time.Sleep(5 * time.Millisecond)
		// Clear file and reset writing offset
		tempfile.Truncate(0)
		tempfile.Seek(0, io.SeekStart)
		writer.Reset(tempfile)
		_, err = writer.WriteString(tc.csv)
		if err != nil {
			t.Error(err)
		}
		err = writer.Flush()
		if err != nil {
			t.Error(err)
		}
		list, err := NewReloadingOwnerList(tempfile.Name())
		if err != nil && !tc.err {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
		if tc.err {
			if err == nil {
				t.Errorf("%s: expected an error", tc.name)
			}
			_, ok := err.(badCsv)
			if !ok {
				t.Errorf("%s: error type is not badCsv: %v", tc.name, err)
			}
			if list == nil {
				t.Errorf("%s: did not return a list during badCsv", tc.name)
			}
		}
		if owner := list.TestOwner(tc.lookup); owner != tc.owner {
			t.Errorf("%s: bad owner %s != %s", tc.name, owner, tc.owner)
		}
		if sig := list.TestSIG(tc.lookup); sig != tc.sig {
			t.Errorf("%s: bad sig %s != %s", tc.name, sig, tc.sig)
		}
	}
}
