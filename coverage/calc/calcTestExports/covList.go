// package calcTestExports stores calc functions for tests,
// used by other packages
package calcTestExports

import (
	"github.com/kubernetes/test-infra/coverage/artifacts/artsTest"
	"github.com/kubernetes/test-infra/coverage/calc"
)

func CovList() *calc.CoverageList {
	arts := artsTest.LocalInputArtsForTest()
	covList := calc.CovList(arts.ProfileReader(), nil, nil, 50)
	covList.Report(true)
	return covList
}
