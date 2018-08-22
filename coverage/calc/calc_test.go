package calc

import (
	"testing"

	"k8s.io/test-infra/coverage/artifacts/artsTest"
	"k8s.io/test-infra/coverage/test"
	"strings"
)

func TestReadLocalProfile(t *testing.T) {
	arts := artsTest.LocalInputArtsForTest()
	covList := CovList(arts.ProfileReader(), nil, nil, 50)
	covList.report(false)
	expected := "56.5%"
	actual := covList.percentage()
	if actual != expected {
		test.Fail(t, "", expected, actual)
	}
}

func covListForTest() *CoverageList {
	arts := artsTest.LocalInputArtsForTest()
	covList := CovList(arts.ProfileReader(), nil, nil, 50)
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