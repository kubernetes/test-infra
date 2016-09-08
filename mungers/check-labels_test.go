/*
Copyright 2015 The Kubernetes Authors.

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

package mungers

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/google/go-github/github"
)

var (
	repoLabels = []*github.Label{
		{Name: stringPtr("team/abc"), Color: stringPtr("d4c5f9")},
		{Name: stringPtr("team/def"), Color: stringPtr("fef2c0")},
		{Name: stringPtr("team/ghi"), Color: stringPtr("c7def8")},
		{Name: stringPtr("label/rep"), Color: stringPtr("c7def8")},
	}
	fileLabels = []*github.Label{
		{Name: stringPtr("release-note"), Color: stringPtr("d4c5f9")},
		{Name: stringPtr("dependency/rkt"), Color: stringPtr("fbfa04")},
		{Name: stringPtr("team/ghi"), Color: stringPtr("c7def8")},
		{Name: stringPtr("label/rep"), Color: stringPtr("def2c1")},
	}
)

type LabelAccessorTest struct {
	AddedLabels []*github.Label
	RepoLabels  []*github.Label
}

func (l *LabelAccessorTest) AddLabel(label *github.Label) error {
	l.AddedLabels = append(l.AddedLabels, label)
	return nil
}

func (l *LabelAccessorTest) GetLabels() ([]*github.Label, error) {
	return l.RepoLabels, nil
}

func mockLabelReader() ([]byte, error) {
	return []byte(`labels:
  - name: release-note
    color: d4c5f9
  - name: dependency/rkt
    color: fbfa04
  - name: team/ghi
    color: c7def8
  - name: label/rep
    color: def2c1`), nil
}

func makeCheckLabelsMunger() *CheckLabelsMunger {
	return &CheckLabelsMunger{
		labelAccessor: &LabelAccessorTest{},
		readFunc:      mockLabelReader,
	}
}

func TestEachLoop(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	tests := []struct {
		name       string
		repoLabels []*github.Label
		expected   []*github.Label
	}{
		{
			name:       "No labels in repository.",
			repoLabels: []*github.Label{},
			expected:   fileLabels,
		},
		{
			name:       "Identical labels in repository and file.",
			repoLabels: fileLabels,
			expected:   []*github.Label{},
		},
		{
			name:       "Adding label with the same name and a different color",
			repoLabels: []*github.Label{repoLabels[3]},
			expected:   fileLabels[0:3],
		},
		{
			name:       "Adding new labels with existing labels in repository.",
			repoLabels: repoLabels,
			expected:   fileLabels[0:2],
		},
	}

	for testNum, test := range tests {
		c := makeCheckLabelsMunger()
		l := &LabelAccessorTest{make([]*github.Label, 0, 0), test.repoLabels}
		c.labelAccessor = l
		c.EachLoop()
		if len(l.AddedLabels) != len(test.expected) {
			t.Errorf("%d:%s: len(expected):%d, len(l.AddedLabels):%d", testNum, test.name, len(test.expected), len(l.AddedLabels))
		}
		for i := 0; i < len(test.expected); i++ {
			if !reflect.DeepEqual(test.expected[i], l.AddedLabels[i]) {
				t.Errorf("%d:%s: missing %v from AddedLabels", testNum, test.name, *test.expected[i].Name)
			}
		}
	}
}
