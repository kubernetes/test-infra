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
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/golang/glog"
)

var tagRegex = regexp.MustCompile(`\[.*?\]|\{.*?\}`)
var whiteSpaceRegex = regexp.MustCompile(`\s+`)

// Turn a test name into a canonical form (without tags, lowercase, etc.)
func normalize(name string) string {
	tagLess := tagRegex.ReplaceAll([]byte(name), []byte(""))
	squeezed := whiteSpaceRegex.ReplaceAll(tagLess, []byte(" "))
	return strings.ToLower(strings.TrimSpace(string(squeezed)))
}

// OwnerInfo stores the SIG and user which have responsibility for the test.
type OwnerInfo struct {
	// User assigned to this test.
	User string
	// SIG holding responsibility for this test.
	SIG string
}

func (o OwnerInfo) String() string {
	return "OwnerInfo{User:'" + o.User + "', SIG:'" + o.SIG + "'}"
}

// OwnerList uses a map to get owners for a given test name.
type OwnerList struct {
	mapping map[string]*OwnerInfo
	rng     *rand.Rand
}

// get returns the Owner for the test with the exact name or the first blob match. Nil is returned
// if none are matched.
func (o *OwnerList) get(testName string) (owner *OwnerInfo) {
	name := normalize(testName)

	// exact mapping
	owner = o.mapping[name]

	// glob matching
	if owner == nil {
		keys := []string{}
		for k := range o.mapping {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if match, _ := filepath.Match(k, name); match {
				owner = o.mapping[k]
				return
			}
		}
	}
	return
}

// TestOwner returns the owner for a test or the empty string if none is found.
func (o *OwnerList) TestOwner(testName string) (owner string) {
	ownerInfo := o.get(testName)
	if ownerInfo != nil {
		owner = ownerInfo.User
	}

	if strings.Contains(owner, "/") {
		ownerSet := strings.Split(owner, "/")
		owner = ownerSet[o.rng.Intn(len(ownerSet))]
	}
	return strings.TrimSpace(owner)
}

// TestSIG returns the SIG assigned to a test, or else the empty string if none is found.
func (o *OwnerList) TestSIG(testName string) string {
	ownerInfo := o.get(testName)
	if ownerInfo == nil {
		return ""
	}
	return strings.TrimSpace(ownerInfo.SIG)
}

// NewOwnerList constructs an OwnerList given a mapping from test names to test owners.
func NewOwnerList(mapping map[string]*OwnerInfo) *OwnerList {
	list := OwnerList{}
	list.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	list.mapping = make(map[string]*OwnerInfo)
	for input, output := range mapping {
		list.mapping[normalize(input)] = output
	}
	return &list
}

// NewOwnerListFromCsv constructs an OwnerList given a CSV file that includes
// 'owner' and 'test name' columns.
func NewOwnerListFromCsv(r io.Reader) (*OwnerList, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	mapping := make(map[string]*OwnerInfo)
	ownerCol := -1
	nameCol := -1
	sigCol := -1
	for _, record := range records {
		if ownerCol == -1 || nameCol == -1 || sigCol == -1 {
			for col, val := range record {
				switch strings.ToLower(val) {
				case "owner":
					ownerCol = col
				case "name":
					nameCol = col
				case "sig":
					sigCol = col
				}

			}
		} else {
			mapping[record[nameCol]] = &OwnerInfo{
				User: record[ownerCol],
				SIG:  record[sigCol],
			}
		}
	}
	if len(mapping) == 0 {
		return nil, errors.New("no mappings found in test owners CSV")
	}
	return NewOwnerList(mapping), nil
}

// ReloadingOwnerList maps test names to owners, reloading the mapping when the
// underlying file is changed.
type ReloadingOwnerList struct {
	path      string
	mtime     time.Time
	ownerList *OwnerList
}

// NewReloadingOwnerList creates a ReloadingOwnerList given a path to a CSV
// file containing owner mapping information.
func NewReloadingOwnerList(path string) (*ReloadingOwnerList, error) {
	ownerList := &ReloadingOwnerList{path: path}
	err := ownerList.reload()
	if err != nil {
		if _, ok := err.(badCsv); !ok {
			return nil, err // Error is not a bad csv file
		}
		glog.Errorf("Unable to load test owners at %s: %v", path, err)
		ownerList.ownerList = NewOwnerList(nil)
	}
	return ownerList, err // err != nil if badCsv (but can recover)
}

// TestOwner returns the owner for a test, or the empty string if none is found.
func (o *ReloadingOwnerList) TestOwner(testName string) string {
	err := o.reload()
	if err != nil {
		glog.Errorf("Unable to reload test owners at %s: %v", o.path, err)
		// Process using the previous data.
	}
	return o.ownerList.TestOwner(testName)
}

// TestSIG returns the SIG for a test, or the empty string if none is found.
func (o *ReloadingOwnerList) TestSIG(testName string) string {
	err := o.reload()
	if err != nil {
		glog.Errorf("Unable to reload test owners at %s: %v", o.path, err)
		// Process using the previous data.
	}
	return o.ownerList.TestSIG(testName)
}

type badCsv string

func (b badCsv) Error() string {
	return string(b)
}

func (o *ReloadingOwnerList) reload() error {
	info, err := os.Stat(o.path)
	if err != nil {
		return err
	}
	if info.ModTime() == o.mtime {
		return nil
	}
	file, err := os.Open(o.path)
	if err != nil {
		return err
	}
	defer file.Close()
	ownerList, err := NewOwnerListFromCsv(file)
	if err != nil {
		return badCsv(fmt.Sprintf("could not parse owner list: %v", err))
	}
	o.ownerList = ownerList
	o.mtime = info.ModTime()
	return nil
}
