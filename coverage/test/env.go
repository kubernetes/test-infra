package test

import (
	"os"
	"path"

	"github.com/sirupsen/logrus"
)

const projectPathLessGoPath = "src/k8s.io/test-infra/coverage"
const projectPathLessTestSrc = "__main__/coverage"

//const projectPathLessGoPath = ""

func ProjDir() string {
	testSrcDir := os.Getenv("TEST_SRCDIR")
	if testSrcDir != "" {
		logrus.Infof("using TEST_SRCDIR=%s", testSrcDir)
		return path.Join(testSrcDir, projectPathLessTestSrc)
	}

	gopath := os.Getenv("GOPATH")
	logrus.Infof("using GOPATH=%s", gopath)
	return path.Join(gopath, projectPathLessGoPath)
}

func absPath(pathRelToProj string) string {
	return path.Join(ProjDir(), pathRelToProj)
}
