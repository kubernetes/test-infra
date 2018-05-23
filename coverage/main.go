/*
Package main calculates and stores code coverage information for given project.
*/
package main

import (
	"flag"
	"fmt"
)

const defaultCoverageProfileName = "coverage_profile.txt"
const defaultCoverageTargetDir = "./target_pkg/"

func main() {

	fmt.Println("entering code coverage main")
	artifactsDir := flag.String("artifacts", "./artifacts", "directory for artifacts")
	coverageTargetDir := flag.String("cov-target", defaultCoverageTargetDir, "target directory for test coverage")
	profileName := flag.String("profileName", defaultCoverageProfileName, "file name for coverage profile")
	pullNumFlag := flag.String("pull-number", "", "PR number")
	pullShaFlag := flag.String("pull-sha", "", "PR commit SHA")
	flag.Parse()

	fmt.Println(*artifactsDir, *coverageTargetDir, *profileName, *pullNumFlag, *pullShaFlag)

	fmt.Println("end of code coverage main")
}
