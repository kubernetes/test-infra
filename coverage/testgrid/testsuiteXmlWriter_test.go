package testgrid

import (
	"k8s.io/test-infra/coverage/artifacts/artsTest"
	"k8s.io/test-infra/coverage/test"
	"testing"
)

const (
	covProfileName = "cov-profile.txt"
	stdoutFileName = "stdout.txt"
)

func TestXMLProduction(t *testing.T) {
	arts := artsTest.LocalArtsForTest("TestXMLProduction")
	test.LinkInputArts(arts.Directory(), covProfileName, stdoutFileName)
	ProfileToTestsuiteXML(arts, 50)
	test.DeleteDir(arts.Directory())
}
