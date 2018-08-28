package git

import (
	"bytes"
	"io"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	gitAttrLinguistGenerated = "linguist-generated"
	gitAttrCoverageExcluded  = "coverage-excluded"
)

// hasGitAttr checks git attribute value exist for the file
func hasGitAttr(attr string, fileName string) bool {
	//fmt.Printf("filename=*%s*\n", fileName)
	attrCmd := exec.Command("git", "check-attr", attr, "--", fileName)
	valCmd := exec.Command("cut", "-d:", "-f", "3")

	pr, pw := io.Pipe()
	attrCmd.Stdout = pw
	valCmd.Stdin = pr
	var val bytes.Buffer
	valCmd.Stdout = &val

	attrCmd.Start()
	valCmd.Start()

	go func() {
		defer pw.Close()
		attrCmd.Wait()
	}()
	valCmd.Wait()

	return strings.ToLower(strings.TrimSpace(val.String())) == "true"
}

//IsCoverageSkipped checks whether a file should be ignored by the code coverage tool
func IsCoverageSkipped(filePath string) bool {
	if hasGitAttr(gitAttrLinguistGenerated, filePath) {
		logrus.Info("Skipping as file is linguist-generated: ", filePath)
		return true
	} else if hasGitAttr(gitAttrCoverageExcluded, filePath) {
		logrus.Info("Skipping as file is coverage-excluded: ", filePath)
		return true
	}
	return false
}
