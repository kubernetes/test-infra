/*
Copyright 2021 The Kubernetes Authors.

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

package slack

import (
	"flag"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestHostsFlag(t *testing.T) {
	var testArg HostsFlag
	flags := flag.NewFlagSet("foo", flag.PanicOnError)
	flags.Var(&testArg, "test-arg", "")
	if err := flags.Parse([]string{"--test-arg", "a=b"}); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(HostsFlag(map[string]string{"a": "b"}), testArg); diff != "" {
		t.Fatalf("Arg parsing mismatch. Want(-), got(+):\n%s", diff)
	}
}
