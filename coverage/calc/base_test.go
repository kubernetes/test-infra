package calc

import (
	"testing"

	"k8s.io/test-infra/coverage/test"
)

func testCoverage() (c *coverage) {
	return &coverage{name: "fake-coverage", nCoveredStmts: 200, nAllStmts: 300, lineCovLink: "fake-line-cov-url"}
}

func TestCoverageRatio(t *testing.T) {
	c := testCoverage()
	actualRatio, _ := c.ratio()
	if actualRatio != float32(c.nCoveredStmts)/float32(c.nAllStmts) {
		t.Fatalf("actualRatio != float32(c.nCoveredStmts) / float32(c.nAllStmts)\n")
	}
}

func TestRatioErr(t *testing.T) {
	c := &coverage{name: "fake-coverage", nCoveredStmts: 200, nAllStmts: 0, lineCovLink: "fake-line-cov-url"}
	_, err := c.ratio()
	if err == nil {
		t.Fatalf("fail to return err for 0 denominator")
	}
}

func TestPercentageNA(t *testing.T) {
	c := &coverage{name: "fake-coverage", nCoveredStmts: 200, nAllStmts: 0, lineCovLink: "fake-line-cov-url"}
	test.AssertEqual(t, "N/A", c.percentage())
}

func TestSort(t *testing.T) {
	cs := []coverage{
		*newCoverage("pear"),
		*newCoverage("apple"),
		*newCoverage("candy"),
	}
	SortCoverages(cs)

	expected := []string{"apple", "candy", "pear"}
	for i, c := range cs {
		test.AssertEqual(t, expected[i], c.Name())
	}
}
