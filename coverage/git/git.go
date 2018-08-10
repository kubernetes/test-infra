package git

import (
	"bytes"
	"io"
	"log"
	"os/exec"
	"strings"
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

	//fmt.Println(strings.ToLower(strings.TrimSpace(val.String())))
	return strings.ToLower(strings.TrimSpace(val.String())) == "true"
}

func IsCoverageSkipped(filePath string) bool {
	if hasGitAttr(gitAttrLinguistGenerated, filePath) {
		log.Println("Skipping as file is linguist-generated: ", filePath)
		return true
	} else if hasGitAttr(gitAttrCoverageExcluded, filePath) {
		log.Println("Skipping as file is coverage-excluded: ", filePath)
		return true
	}
	return false
}
