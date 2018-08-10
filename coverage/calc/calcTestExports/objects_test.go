package calcTestExports

import (
	"strings"
	"testing"
)

func TestCovList(t *testing.T) {
	l := CovList()
	if len(*l.Group()) == 0 {
		t.Fatalf("covlist is empty\n")
	}
	if !strings.HasSuffix(l.Percentage(), "%") {
		t.Fatalf("covlist.Percentage() doesn't end with %%\n")
	}
}
