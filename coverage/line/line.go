// line by line coverage
package line

import (
	"fmt"
	"github.com/kubernetes/test-infra/coverage/artifacts"
	"github.com/kubernetes/test-infra/coverage/calc"
	"github.com/kubernetes/test-infra/coverage/gcs"
	"log"
	"os/exec"
)

func CreateLineCovFile(arts *artifacts.LocalArtifacts) error {
	pathKeyProfile := arts.KeyProfilePath()
	pathLineCov := arts.LineCovFilePath()
	cmdTxt := fmt.Sprintf("go tool cover -html=%s -o %s", pathKeyProfile, pathLineCov)
	log.Printf("Running command '%s'\n", cmdTxt)
	cmd := exec.Command("go", "tool", "cover", "-html="+pathKeyProfile, "-o", pathLineCov)
	log.Printf("Finished running '%s'\n", cmdTxt)
	err := cmd.Run()
	log.Printf("cmd.Args=%v", cmd.Args)
	if err != nil {
		log.Printf("Error executing cmd: %v", err)
	}
	return err
}

func GenerateLineCovLinks(
	presubmitBuild *gcs.PreSubmit, g *calc.CoverageList) {
	calc.SortCoverages(*g.Group())
	for i := 0; i < len(*g.Group()); i++ {
		g.Item(i).SetLineCovLink(presubmitBuild.UrlGcsLineCovLinkWithMarker(i))
		fmt.Printf("g.Item(i=%d).LineCovLink(): %s\n", i, g.Item(i).LineCovLink())
	}
}
