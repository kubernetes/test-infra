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

package options

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	str1, str2     string
	slice1, slice2 []string
	int1           int
	uint1          uint64
	bool1          bool
	bool2          bool
	dur1           time.Duration

	entries = []optEntry{
		{key: "strvar", val: "def", ptr: &str1},
		{key: "strslicevar", val: []string{"def", "def2"}, ptr: &slice1},
		{key: "intvar", val: 5, ptr: &int1},
		{key: "uintvar", val: uint64(5), ptr: &uint1},
		{key: "boolvar", val: true, ptr: &bool1},
		{key: "boolvar2", val: false, ptr: &bool2},
		{key: "durvar", val: time.Second * 2, ptr: &dur1},
		{key: "emptyslicevar", val: []string{"def"}, ptr: &slice2},
		{key: "emptystringvar", val: "def", ptr: &str2},
	}

	sample_yaml = `strvar: Hello world.
strslicevar: Hello world., string2
intvar: 5
uintvar: 6
boolvar: false
boolvar2: true
durvar: 15s
emptyslicevar:
emptystringvar: ""`
	expected = []optEntry{
		{key: "strvar", val: "Hello world.", ptr: &str1},
		{key: "strslicevar", val: []string{"Hello world.", "string2"}, ptr: &slice1},
		{key: "intvar", val: 5, ptr: &int1},
		{key: "uintvar", val: uint64(6), ptr: &uint1},
		{key: "boolvar", val: false, ptr: &bool1},
		{key: "boolvar2", val: true, ptr: &bool2},
		{key: "durvar", val: time.Second * 15, ptr: &dur1},
		{key: "emptyslicevar", val: []string{}, ptr: &slice2},
		{key: "emptystringvar", val: "", ptr: &str2},
	}

	sample_yaml2 = `intvar: 700
durvar: 5s
emptyslicevar: ""`
	expected2 = []optEntry{
		{key: "strvar", val: "def", ptr: &str1},
		{key: "strslicevar", val: []string{"def", "def2"}, ptr: &slice1},
		{key: "intvar", val: 700, ptr: &int1},
		{key: "uintvar", val: uint64(5), ptr: &uint1},
		{key: "boolvar", val: true, ptr: &bool1},
		{key: "boolvar2", val: false, ptr: &bool2},
		{key: "durvar", val: time.Second * 5, ptr: &dur1},
		{key: "emptyslicevar", val: []string{}, ptr: &slice2},
		{key: "emptystringvar", val: "def", ptr: &str2},
	}
)

type tempConfig struct {
	file   *os.File
	writer *bufio.Writer
}

func newTempConfig() (*tempConfig, error) {
	tempfile, err := ioutil.TempFile(os.TempDir(), "options_test")
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

type optEntry struct {
	key string
	val interface{}
	ptr interface{}
}

func registerAll(opts *Options) {
	for _, entry := range entries {
		switch defVal := entry.val.(type) {
		case string:
			opts.RegisterString(entry.ptr.(*string), entry.key, defVal, "Desc: "+entry.key)
		case []string:
			opts.RegisterStringSlice(entry.ptr.(*[]string), entry.key, defVal, "Desc: "+entry.key)
		case int:
			opts.RegisterInt(entry.ptr.(*int), entry.key, defVal, "Desc: "+entry.key)
		case uint64:
			opts.RegisterUint64(entry.ptr.(*uint64), entry.key, defVal, "Desc: "+entry.key)
		case bool:
			opts.RegisterBool(entry.ptr.(*bool), entry.key, defVal, "Desc: "+entry.key)
		case time.Duration:
			opts.RegisterDuration(entry.ptr.(*time.Duration), entry.key, defVal, "Desc: "+entry.key)
		}
	}
}

func checkAll(opts *Options, expected []optEntry) error {
	for _, entry := range expected {
		switch expectedVal := entry.val.(type) {
		case string:
			v := opts.GetString(entry.key)
			if v != entry.ptr {
				return fmt.Errorf("string opt '%s' moved! Was at %p, is now at %p", entry.key, entry.ptr, v)
			}
			if *v != expectedVal {
				return fmt.Errorf("string opt '%s' has value %q, expected %q", entry.key, *v, expectedVal)
			}
		case []string:
			v := opts.GetStringSlice(entry.key)
			if v != entry.ptr {
				return fmt.Errorf("[]string opt '%s' moved! Was at %p, is now at %p", entry.key, entry.ptr, v)
			}
			if !reflect.DeepEqual(*v, expectedVal) {
				return fmt.Errorf("[]string opt '%s' has value %q, expected %q", entry.key, *v, expectedVal)
			}
		case int:
			v := opts.GetInt(entry.key)
			if v != entry.ptr {
				return fmt.Errorf("int opt '%s' moved! Was at %p, is now at %p", entry.key, entry.ptr, v)
			}
			if *v != expectedVal {
				return fmt.Errorf("int opt '%s' has value %d, expected %d", entry.key, *v, expectedVal)
			}
		case uint64:
			v := opts.GetUint64(entry.key)
			if v != entry.ptr {
				return fmt.Errorf("uint64 opt '%s' moved! Was at %p, is now at %p", entry.key, entry.ptr, v)
			}
			if *v != expectedVal {
				return fmt.Errorf("uint64 opt '%s' has value %d, expected %d", entry.key, *v, expectedVal)
			}
		case bool:
			v := opts.GetBool(entry.key)
			if v != entry.ptr {
				return fmt.Errorf("bool opt '%s' moved! Was at %p, is now at %p", entry.key, entry.ptr, v)
			}
			if *v != expectedVal {
				return fmt.Errorf("bool opt '%s' has value %v, expected %v", entry.key, *v, expectedVal)
			}
		case time.Duration:
			v := opts.GetDuration(entry.key)
			if v != entry.ptr {
				return fmt.Errorf("duration opt '%s' moved! Was at %p, is now at %p", entry.key, entry.ptr, v)
			}
			if *v != expectedVal {
				return fmt.Errorf("duration opt '%s' has value %v, expected %v", entry.key, *v, expectedVal)
			}
		}
	}
	return nil
}

// TestOptions test that all option types are recalled properly.
// Specifically, options are checked to be correct after various combinations and orderings of
// loads, registers, reloads, and reregisters.
func TestOptions(t *testing.T) {
	cfg, err := newTempConfig()
	if err != nil {
		t.Error(err)
	}
	defer cfg.Clean()

	if err = cfg.SetContent(sample_yaml); err != nil {
		t.Error(err)
	}
	// Test simple load + register in both orders.
	oLoadFirst := New()
	oRegisterFirst := New()
	oRegisterOnce := New()

	if _, err = oLoadFirst.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}
	registerAll(oLoadFirst)
	registerAll(oRegisterFirst)
	if _, err = oRegisterFirst.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}
	//Test Register only (no Load).
	registerAll(oRegisterOnce)
	if err = checkAll(oRegisterOnce, entries); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}
	if _, err = oRegisterOnce.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}

	if err = checkAll(oLoadFirst, expected); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}
	if err = checkAll(oRegisterFirst, expected); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}
	if err = checkAll(oRegisterOnce, expected); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}

	if err = cfg.SetContent(sample_yaml2); err != nil {
		t.Error(err)
	}
	// Test back to back loads and registers.
	registerAll(oLoadFirst)
	if _, err = oLoadFirst.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}
	if _, err = oRegisterFirst.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}
	registerAll(oRegisterFirst)
	// Test reload without reregister.
	if _, err = oRegisterOnce.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}

	if err = checkAll(oLoadFirst, expected2); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}
	if err = checkAll(oRegisterFirst, expected2); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}
	if err = checkAll(oRegisterOnce, expected2); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}

	// Test no-op reload of same config (sample_yaml2 again).
	if _, err = oRegisterOnce.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}
	if err = checkAll(oRegisterOnce, expected2); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}
}

// TestDefaults verifies that Options use their default values when a value is not specified.
func TestDefaults(t *testing.T) {
	cfg, err := newTempConfig()
	if err != nil {
		t.Error(err)
	}
	defer cfg.Clean()

	if err = cfg.SetContent(sample_yaml2); err != nil {
		t.Error(err)
	}

	// Test Load then Register.
	oUseDefaults := New()
	if _, err = oUseDefaults.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}
	registerAll(oUseDefaults)

	if err = checkAll(oUseDefaults, expected2); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}
	// Test Register then Load.
	oUseDefaults = New()
	registerAll(oUseDefaults)
	if _, err = oUseDefaults.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}
	if err = checkAll(oUseDefaults, expected2); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}

	// Test that an option reverts back to default if a value is no longer specified after a reload.
	if err = cfg.SetContent(sample_yaml); err != nil {
		t.Error(err)
	}
	if _, err = oUseDefaults.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}
	if err = checkAll(oUseDefaults, expected); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}
	if err = cfg.SetContent(sample_yaml2); err != nil {
		t.Error(err)
	}
	if _, err = oUseDefaults.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}
	if err = checkAll(oUseDefaults, expected2); err != nil {
		t.Errorf("checkAll failed: %v", err)
	}
}

func TestDescriptionsAndToString(t *testing.T) {
	cfg, err := newTempConfig()
	if err != nil {
		t.Error(err)
	}
	defer cfg.Clean()

	if err = cfg.SetContent(sample_yaml); err != nil {
		t.Error(err)
	}

	o := New()
	registerAll(o)
	desc := o.Descriptions()
	for _, entry := range entries {
		if !strings.Contains(desc, "\"Desc: "+entry.key+"\"") ||
			!strings.Contains(desc, entry.key) {
			t.Errorf("Description is malformed for key '%s':\n%s\n", entry.key, desc)
		}
	}
	// Check that default values are formatted as expected.
	checks := []string{"(\"def\")", "([\"def\", \"def2\"])", "(2s)", "(5)", "(true)"}
	for _, check := range checks {
		if !strings.Contains(desc, check) {
			t.Errorf("Description does not contain default value '%s':\n%s\n", check, desc)
		}
	}
}

func TestUpdateCallback(t *testing.T) {
	cfg, err := newTempConfig()
	if err != nil {
		t.Error(err)
	}
	defer cfg.Clean()

	if err = cfg.SetContent(sample_yaml); err != nil {
		t.Error(err)
	}

	o := New()
	registerAll(o)
	o.RegisterUpdateCallback(func(changed sets.String) error {
		return fmt.Errorf("some error")
	})
	if _, err = o.Load(cfg.file.Name()); err != nil {
		if _, ok := err.(*UpdateCallbackError); ok {
			t.Errorf("Unexpected UpdateCallbackError from first load (should not have called callback): %v", err)
		} else {
			t.Errorf("Load failed: %v", err)
		}
	}

	if err = cfg.SetContent(sample_yaml2); err != nil {
		t.Error(err)
	}
	if _, err = o.Load(cfg.file.Name()); err == nil {
		t.Errorf("Expected an UpdateCallbackError but no error was returned.")
	} else if _, ok := err.(*UpdateCallbackError); !ok {
		t.Errorf("Expected an UpdateCallbackError but got a different error: %v", err)
	}
}

func TestChanged(t *testing.T) {
	cfg, err := newTempConfig()
	if err != nil {
		t.Error(err)
	}
	defer cfg.Clean()

	if err = cfg.SetContent(sample_yaml); err != nil {
		t.Error(err)
	}

	o := New()
	registerAll(o)
	var changed sets.String
	if changed, err = o.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}

	if err = cfg.SetContent(sample_yaml2); err != nil {
		t.Error(err)
	}
	expectedChanges := sets.NewString("durvar", "intvar", "strvar", "strslicevar", "uintvar", "boolvar", "boolvar2", "emptystringvar")
	if changed, err = o.Load(cfg.file.Name()); err != nil {
		t.Errorf("Load failed: %v", err)
	}
	if !changed.Equal(expectedChanges) {
		t.Errorf(
			"Error: expected options %q to be changed, but %q were changed.",
			expectedChanges.List(),
			changed.List(),
		)
	}
}
