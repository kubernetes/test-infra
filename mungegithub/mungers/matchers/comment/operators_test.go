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

package comment

import "testing"

func TestTrue(t *testing.T) {
	if !(True{}).Match(nil) {
		t.Error("True shouldn't be false ...")
	}
}

func TestFalse(t *testing.T) {
	if (False{}).Match(nil) {
		t.Error("False shouldn't be true ...")
	}
}

func TestNot(t *testing.T) {
	if !(Not{False{}}).Match(nil) {
		t.Error("Not False shouldn't be false ...")
	}

	if (Not{True{}}).Match(nil) {
		t.Error("Not True shouldn't be true ...")
	}
}

func TestAnd(t *testing.T) {
	if !(And{}).Match(nil) {
		t.Error("And(Empty) should be true ...")
	}

	if !(And([]Matcher{True{}, True{}})).Match(nil) {
		t.Error("And(All True) should be true ...")
	}

	if (And([]Matcher{True{}, False{}})).Match(nil) {
		t.Error("And(Some False) should be false ...")
	}

	if (And([]Matcher{False{}, False{}})).Match(nil) {
		t.Error("And(All False) should be false ...")
	}
}

func TestOr(t *testing.T) {
	if (Or{}).Match(nil) {
		t.Error("Or(Empty) should be false ...")
	}

	if !(Or([]Matcher{True{}, True{}})).Match(nil) {
		t.Error("Or(All True) should be true ...")
	}

	if !(Or([]Matcher{True{}, False{}})).Match(nil) {
		t.Error("Or(Some False) should be true ...")
	}

	if (Or([]Matcher{False{}, False{}})).Match(nil) {
		t.Error("Or(All False) should be false ...")
	}
}
