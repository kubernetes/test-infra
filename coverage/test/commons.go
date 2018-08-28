package test

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

// StrFailure is used to display discrepancy between expected and actual result in test
func StrFailure(input, expected, actual string) string {
	return fmt.Sprintf("input=%s; expected=%s; actual=%s\n", input, expected, actual)
}

//Fail fails a test and prints out info about expected and actual value
func Fail(t *testing.T, input, expected, actual interface{}) {
	t.Fatalf("input=%s; expected=%v; actual=%v\n", input, expected, actual)
}

//AssertEqual checks equality of expected and actual results, fail the test if not equal
func AssertEqual(t *testing.T, expected, actual interface{}) {
	if expected != actual {
		t.Fatalf("expected='%v'; actual='%v'\n", expected, actual)
	}
}

//FileOrDirExists checks the existence of given file or directory
func FileOrDirExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			cwd, _ := os.Getwd()
			logrus.Infof("file or dir not found: %s; cwd=%s", path, cwd)
			return false
		}
		log.Fatalf("File stats (path=%s) err: %v", path, err)
	}
	return true
}

type stringSet struct {
	data map[string]bool
}

//Add adds a string to the string set
func (set *stringSet) Add(s string) {
	set.data[s] = true
}

//Has checks if the string is a member of the string set
func (set *stringSet) Has(s string) bool {
	return set.data[s]
}

func newStringSet() *stringSet {
	return &stringSet{
		data: make(map[string]bool),
	}
}

func MakeStringSet(members ...string) (set *stringSet) {
	set = newStringSet()
	for _, member := range members {
		set.Add(member)
	}
	return set
}

func (set *stringSet) AllMembers() (res []string) {
	for item, valid := range set.data {
		if valid {
			res = append(res, item)
		}
	}
	return
}
