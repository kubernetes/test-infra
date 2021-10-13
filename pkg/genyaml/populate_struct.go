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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/sirupsen/logrus"
)

// PopulateStruct will recursively populate a struct via reflection for consumption by genyaml by:
// * Filling all pointer fields
// * Filling all slices with a one-element slice and filling that one element
// * Filling all maps with a one-element map and filling that one element
// NOTE: PopulateStruct will panic if not fed a pointer. Generally if you care about
// the stability of the app that runs this code, it is strongly recommended to recover panics:
// defer func(){if r := recover(); r != nil { fmt.Printf("Recovered panic: %v", r } }(
func PopulateStruct(in interface{}) interface{} {
	typeOf := reflect.TypeOf(in)
	if typeOf.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("got nonpointer type %T", in))
	}
	if typeOf.Elem().Kind() != reflect.Struct {
		return in
	}
	valueOf := reflect.ValueOf(in)
	for i := 0; i < typeOf.Elem().NumField(); i++ {
		// Unexported
		if !valueOf.Elem().Field(i).CanSet() {
			continue
		}

		if typeOf.Elem().Field(i).Anonymous {
			// We can only warn about this, because the go stdlib and some kube types do this :/
			if !strings.Contains(typeOf.Elem().Field(i).Tag.Get("json"), ",inline") {
				logrus.Warningf("Found anonymous field without inline annotation, this will produce invalid results. Please add a `json:\",inline\"` annotation if you control the type. Type: %T, parentType: %T", valueOf.Elem().Field(i).Interface(), valueOf.Elem().Interface())
			}
		}
		switch k := typeOf.Elem().Field(i).Type.Kind(); k {
		// We must populate strings, because genyaml uses a custom json lib
		// that omits structs that have empty values only and we have some
		// structs that only have string fields
		// TODO: Is that genyaml behavior intentional?
		case reflect.String:
			// ArrayOrString comes from Tekton and has a custom marshaller
			// that errors when only setting the String field
			if typeOf.Elem().Name() == "ArrayOrString" {
				valueOf.Elem().FieldByName("Type").SetString("string")
			}
			valueOf.Elem().Field(i).SetString(" ")
		// Needed because of the String field handling
		case reflect.Struct:
			PopulateStruct(valueOf.Elem().Field(i).Addr().Interface())
		case reflect.Ptr:
			ptr := createNonNilPtr(valueOf.Elem().Field(i).Type())
			// Populate our ptr
			if ptr.Elem().Kind() == reflect.Struct {
				PopulateStruct(ptr.Interface())
			}
			// Set it on the parent struct
			valueOf.Elem().Field(i).Set(ptr)
		case reflect.Slice:
			if typeOf.Elem().Field(i).Type == typeOfBytes || typeOf.Elem().Field(i).Type == typeOfJSONRawMessage {
				continue
			}
			// Create a one element slice
			slice := reflect.MakeSlice(typeOf.Elem().Field(i).Type, 1, 1)
			// Get a pointer to the value
			var sliceElementPtr interface{}
			if slice.Index(0).Type().Kind() == reflect.Ptr {
				// Slice of pointers, make it a non-nil pointer, then pass on its address
				slice.Index(0).Set(createNonNilPtr(slice.Index(0).Type()))
				sliceElementPtr = slice.Index(0).Interface()
			} else {
				// Slice of literals
				sliceElementPtr = slice.Index(0).Addr().Interface()
			}
			PopulateStruct(sliceElementPtr)
			// Set it on the parent struct
			valueOf.Elem().Field(i).Set(slice)
		case reflect.Map:
			keyType := typeOf.Elem().Field(i).Type.Key()
			valueType := typeOf.Elem().Field(i).Type.Elem()

			key := reflect.New(keyType).Elem()
			value := reflect.New(valueType).Elem()

			var keyPtr, valPtr interface{}
			if key.Kind() == reflect.Ptr {
				key.Set(createNonNilPtr(key.Type()))
				keyPtr = key.Interface()
			} else {
				keyPtr = key.Addr().Interface()
			}
			if value.Kind() == reflect.Ptr {
				value.Set(createNonNilPtr(value.Type()))
				valPtr = value.Interface()
			} else {
				valPtr = value.Addr().Interface()
			}
			PopulateStruct(keyPtr)
			PopulateStruct(valPtr)

			mapType := reflect.MapOf(typeOf.Elem().Field(i).Type.Key(), typeOf.Elem().Field(i).Type.Elem())
			concreteMap := reflect.MakeMapWithSize(mapType, 0)
			concreteMap.SetMapIndex(key, value)

			valueOf.Elem().Field(i).Set(concreteMap)
		}

	}
	return in
}

func createNonNilPtr(in reflect.Type) reflect.Value {
	// construct a new **type and call Elem() to get the *type
	ptr := reflect.New(in).Elem()
	// Give it a value
	ptr.Set(reflect.New(ptr.Type().Elem()))

	return ptr
}

var typeOfBytes = reflect.TypeOf([]byte(nil))
var typeOfJSONRawMessage = reflect.TypeOf(json.RawMessage{})
