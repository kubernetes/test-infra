package calc

import (
	"github.com/kubernetes/test-infra/coverage/artifacts/artsTest"
	"github.com/kubernetes/test-infra/coverage/test"
	"testing"
)

func TestReadLocalProfile(t *testing.T) {
	arts := artsTest.LocalInputArtsForTest()
	covList := CovList(arts.ProfileReader(), nil, nil, 50)
	covList.Report(false)
	expected := "56.5%"
	actual := covList.Percentage()
	if actual != expected {
		test.Fail(t, "", expected, actual)
	}
}
