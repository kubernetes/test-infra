/*
Copyright 2018 The Kubernetes Authors.

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

package calc

import (
	"testing"

	"k8s.io/test-infra/coverage/artifacts/artsTest"
	"k8s.io/test-infra/coverage/test"
	"strings"
)

func TestReadLocalProfile(t *testing.T) {
	arts := artsTest.LocalInputArtsForTest()
	covList := CovList(arts.ProfileReader(), nil, false, &map[string]bool{}, 50)
	covList.report(false)
	expected := "56.5%"
	actual := covList.percentage()
	if actual != expected {
		test.Fail(t, "", expected, actual)
	}
}

func covListForTest() *CoverageList {
	arts := artsTest.LocalInputArtsForTest()
	covList := CovList(arts.ProfileReader(), nil, false, &map[string]bool{}, 50)
	covList.report(true)
	return covList
}

func TestCovList(t *testing.T) {
	l := covListForTest()
	if len(*l.Group()) == 0 {
		t.Fatalf("covlist is empty\n")
	}
	if !strings.HasSuffix(l.percentage(), "%") {
		t.Fatalf("covlist.Percentage() doesn't end with %%\n")
	}
}
