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
