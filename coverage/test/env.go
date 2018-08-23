package test

import (
	"os"
	"path"
)

const projectPathLessGoPath = "src/k8s.io/test-infra/coverage"

func ProjDir() string {
	gopath := os.Getenv("GOPATH")
	return path.Join(gopath, projectPathLessGoPath)
}

func absPath(pathRelToProj string) string {
	return path.Join(ProjDir(), pathRelToProj)
}
