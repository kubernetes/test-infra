// line by line coverage
package line

import (
	"fmt"
	"os/exec"

	"github.com/sirupsen/logrus"

	"errors"
	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/calc"
	"k8s.io/test-infra/coverage/gcs"
	"k8s.io/test-infra/coverage/io"
)

//CreateLineCovFile creates the html file of line-by-line coverage
func CreateLineCovFile(arts *artifacts.LocalArtifacts) error {
	pathKeyProfile := arts.KeyProfilePath()
	pathLineCov := arts.LineCovFilePath()
	cmdTxt := fmt.Sprintf("go tool cover -html=%s -o %s", pathKeyProfile, pathLineCov)

	if !io.FileOrDirExists(pathKeyProfile) {
		logrus.Infof("key profile not found on path=%s", pathKeyProfile)
		return errors.New("key profile not found")
	}

	logrus.Infof("Running command '%s'\n", cmdTxt)
	cmd := exec.Command("go", "tool", "cover", "-html="+pathKeyProfile, "-o", pathLineCov)
	logrus.Infof("Finished running '%s'\n", cmdTxt)
	err := cmd.Run()
	logrus.Infof("cmd.Args=%v", cmd.Args)
	if err != nil {
		logrus.Infof("Error executing cmd, err = %v", err)
	}
	return err
}

//GenerateLineCovLinks adds line coverage link to each coverage object in the CoverageList
func GenerateLineCovLinks(
	presubmitBuild *gcs.PreSubmit, g *calc.CoverageList) {
	calc.SortCoverages(*g.Group())
	for i := 0; i < len(*g.Group()); i++ {
		g.Item(i).SetLineCovLink(presubmitBuild.UrlGcsLineCovLinkWithMarker(i))
		fmt.Printf("g.Item(i=%d).LineCovLink(): %s\n", i, g.Item(i).LineCovLink())
	}
}
