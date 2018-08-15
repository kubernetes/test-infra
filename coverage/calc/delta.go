package calc

import (
	"fmt"
	"k8s.io/test-infra/coverage/githubUtil"
	"k8s.io/test-infra/coverage/str"
	"log"
	"os"
	"sort"
	"strings"
)

type Incremental struct {
	base Coverage
	new  Coverage
}

func (inc Incremental) delta() float32 {
	baseRatio, _ := inc.base.Ratio()
	newRatio, _ := inc.new.Ratio()
	return newRatio - baseRatio
}

func (inc Incremental) Delta() string {
	return str.PercentStr(inc.delta())
}

func (inc Incremental) deltaForCovbot() string {
	if inc.base.nAllStmts == 0 {
		return ""
	}
	return str.PercentageForCovbotDelta(inc.delta())
}

func (inc Incremental) oldCovForCovbot() string {
	if inc.base.nAllStmts == 0 {
		return "Do not exist"
	}
	return inc.base.Percentage()
}

func (inc Incremental) String() string {
	return fmt.Sprintf("<%s> (%d / %d) %s ->(%d / %d) %s", inc.base.Name(),
		inc.base.nCoveredStmts, inc.base.nAllStmts, inc.base.Percentage(),
		inc.new.nCoveredStmts, inc.new.nAllStmts, inc.new.Percentage())
}

type GroupChanges struct {
	Added     []Coverage
	Deleted   []Coverage
	Unchanged []Coverage
	Changed   []Incremental
	BaseGroup *CoverageList
	NewGroup  *CoverageList
}

func sorted(m map[string]Coverage) (result []Coverage) {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		result = append(result, m[k])
	}
	return
}

// NewGroupChanges compares the newList of coverage against the base list and
// returns the result
func NewGroupChanges(baseList *CoverageList, newList *CoverageList) *GroupChanges {
	var added, unchanged []Coverage
	var changed []Incremental
	baseFilesMap := baseList.Map()
	for _, newCov := range newList.group {
		newCovName := newCov.Name()
		baseCov, ok := baseFilesMap[newCovName]
		isNewFile := false
		if !ok {
			added = append(added, newCov)
			baseCov = *newCoverage(newCovName)
			isNewFile = true
		}

		// after all the deletions, the leftover would be the elements that only exists in base group,
		// in other words, the files that is deleted in the new group
		delete(baseFilesMap, newCovName)

		incremental := Incremental{baseCov, newCov}
		delta := incremental.delta()
		if delta == 0 && !isNewFile {
			unchanged = append(unchanged, newCov)
		} else {
			changed = append(changed, incremental)
		}
	}

	return &GroupChanges{Added: added, Deleted: sorted(baseFilesMap), Unchanged: unchanged,
		Changed: changed, BaseGroup: baseList, NewGroup: newList}
}

// processChangedFiles checks each entry in GroupChanges and see if it is
// include in the github commit. If yes, then include that in the covbot report
func (changes *GroupChanges) processChangedFiles(
	githubFilePaths *map[string]bool, rows *[]string, isEmpty,
	isCoverageLow *bool) {
	log.Printf("\nFinding joining set of changed files from profile[count=%d"+
		"] & github\n", len(changes.Changed))
	covThres := changes.NewGroup.covThresholdInt
	for i, inc := range changes.Changed {
		pathFromProfile := githubUtil.FilePathProfileToGithub(inc.base.Name())
		fmt.Printf("checking if this file is in github change list: %s", pathFromProfile)
		if (*githubFilePaths)[pathFromProfile] == true {
			fmt.Printf("\tYes!\n")
			*rows = append(*rows, inc.githubBotRow(i, pathFromProfile))
			*isEmpty = false

			if inc.new.IsCoverageLow(covThres) {
				*isCoverageLow = true
			}
		} else {
			fmt.Printf("\tNo\n")
		}
	}
	fmt.Println("End of Finding joining set of changed files from profile & github")
	return
}

func (inc Incremental) filePathWithHyperlink(filepath string) string {
	return fmt.Sprintf("[%s](%s)", filepath, inc.new.lineCovLink)
}

// githubBotRow returns a string as the content of a row covbot posts
func (inc Incremental) githubBotRow(index int, filepath string) string {
	return fmt.Sprintf("%s | %s | %s | %s",
		inc.filePathWithHyperlink(filepath), inc.oldCovForCovbot(),
		inc.new.Percentage(), inc.deltaForCovbot())
}

// ContentForGithubPost constructs the message covbot posts
func (changes *GroupChanges) ContentForGithubPost(files *map[string]bool) (
	res string, isEmpty, isCoverageLow bool) {
	jobName := os.Getenv("JOB_NAME")
	rows := []string{
		"The following is the coverage report on pkg/.",
		fmt.Sprintf("Say `/test %s` to re-run this coverage report", jobName),
		"",
		"File | Old Coverage | New Coverage | Delta",
		"---- |:------------:|:------------:|:-----:",
	}

	fmt.Printf("\n%d files changed, reported by github:\n", len(*files))
	for githubFilePath := range *files {
		fmt.Printf("%s\t", githubFilePath)
	}
	fmt.Printf("\n\n")

	isEmpty = true
	isCoverageLow = false

	changes.processChangedFiles(files, &rows, &isEmpty, &isCoverageLow)

	rows = append(rows, "")

	return strings.Join(rows, "\n"), isEmpty, isCoverageLow
}
