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

package genyaml

import (
	"testing"
)

func TestPopulateStructHandlesPointerFields(t *testing.T) {
	s := struct {
		Field *struct {
			NestedField *int
		}
	}{}

	PopulateStruct(&s)
	if s.Field == nil {
		t.Fatalf("Pointer field in struct didn't get set, struct: %+v", s)
	}
	if s.Field.NestedField == nil {
		t.Fatalf("Nested pointer field in struct didn't get set, struct: %+v", s)
	}
}

func TestPopulateStructHandlesSlicesOfLiterals(t *testing.T) {
	s := struct {
		Field []struct {
			NestedField *int
		}
	}{}

	PopulateStruct(&s)
	if n := len(s.Field); n != 1 {
		t.Fatalf("Slice didn't get populated, struct: %+v", s)
	}

	if s.Field[0].NestedField == nil {
		t.Fatalf("Slice element field didn't get populated, struct: %+v", s)
	}
}

func TestPopulateStructHandlesSliceOfPointers(t *testing.T) {
	s := struct {
		Field []*struct {
			NestedField *int
		}
	}{}

	PopulateStruct(&s)
	if n := len(s.Field); n != 1 {
		t.Fatalf("Slice didn't get populated, struct: %+v", s)
	}
	if s.Field[0] == nil {
		t.Fatalf("Slice element didn't get populated, struct: %+v", s)
	}

	if s.Field[0].NestedField == nil {
		t.Fatalf("Slice element field didn't get populated, struct: %+v", s)
	}
}

func TestPopulateStructHandlesMaps(t *testing.T) {
	s := struct {
		Field map[string]struct {
			NestedField *int
		}
	}{}

	PopulateStruct(&s)
	if n := len(s.Field); n != 1 {
		t.Fatalf("Map didn't get populated, struct: %+v", s)
	}

	for _, v := range s.Field {
		if v.NestedField == nil {
			t.Fatalf("Pointer field in map element didn't get populated, struct: %+v", s)
		}
	}
}

func TestPopulateStructHandlesMapsWithPointerValues(t *testing.T) {
	s := struct {
		Field map[string]*struct {
			NestedField *int
		}
	}{}

	PopulateStruct(&s)
	if n := len(s.Field); n != 1 {
		t.Fatalf("Map didn't get populated, struct: %+v", s)
	}

	for _, v := range s.Field {
		if v == nil {
			t.Fatalf("Map value is a nilpointer, struct: %+v", s)
		}
		if v.NestedField == nil {
			t.Fatalf("Pointer field in map element didn't get populated, struct: %+v", s)
		}
	}
}

func TestPopulateStructHandlesMapsWithPointerKeys(t *testing.T) {
	s := struct {
		Field map[*string]struct {
			NestedField *int
		}
	}{}

	PopulateStruct(&s)
	if n := len(s.Field); n != 1 {
		t.Fatalf("Map didn't get populated, struct: %+v", s)
	}

	for k, v := range s.Field {
		if k == nil {
			t.Fatalf("Map key is a nilpointer, struct: %+v", s)
		}
		if v.NestedField == nil {
			t.Fatalf("Pointer field in map element didn't get populated, struct: %+v", s)
		}
	}
}

// this is needed because genyaml uses a custom json unmarshaler that omits empty structs
// even if they are not pointers. If we don't do this, structs that only have string fields
// end up being omitted.
// TODO: Is there a necessity for genyaml to have this custom unmarshaler?
func TestPopulateStructSetsStrings(t *testing.T) {
	s := struct {
		String string
		Field  struct {
			String string
		}
	}{}

	PopulateStruct(&s)
	if s.String == "" {
		t.Errorf("String field didn't get set, struct: %+v", s)
	}
	if s.Field.String == "" {
		t.Errorf("Nested string field didn't get set, struct: %+v", s)
	}
}

func TestPopulateStructHandlesUnexportedFields(_ *testing.T) {
	s := struct {
		unexported *struct {
			justAsUnexported *int
		}
	}{}

	PopulateStruct(&s)
	// This test indicates success by not panicking, so nothing further to check
}
