package test

import (
	"fmt"
	"log"
	"os"
	"testing"
)

// StrFailure is used to display discrepancy between expected and actual result in test
func StrFailure(input, expected, actual string) string {
	return fmt.Sprintf("input=%s; expected=%s; actual=%s\n", input, expected, actual)
}

func Fail(t *testing.T, input, expected, actual interface{}) {
	t.Fatalf("input=%s; expected=%v; actual=%v\n", input, expected, actual)
}

func AssertEqual(t *testing.T, expected, actual interface{}) {
	if expected != actual {
		t.Fatalf("expected='%v'; actual='%v'\n", expected, actual)
	}
}

func FileOrDirExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			cwd, _ := os.Getwd()
			log.Printf("file or dir not found: %s; cwd=%s", path, cwd)
			return false
		}
		log.Fatalf("File stats (path=%s) err: %v", path, err)
	}
	return true
}

type Set interface {
	Add(interface{})
	Has(interface{}) bool
}

type StringSet struct {
	data map[string]bool
}

func (set *StringSet) Add(s string) {
	set.data[s] = true
}

func (set *StringSet) Has(s string) bool {
	return set.data[s]
}

func NewStringSet() *StringSet {
	return &StringSet{
		data: make(map[string]bool),
	}
}

func MakeStringSet(members ...string) (set *StringSet) {
	set = NewStringSet()
	for _, member := range members {
		set.Add(member)
	}
	return set
}

func (set *StringSet) AllMembers() (res []string) {
	for item, valid := range set.data {
		if valid {
			res = append(res, item)
		}
	}
	return
}
