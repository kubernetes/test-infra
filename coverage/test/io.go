package test

import (
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/sirupsen/logrus"
)

func DeleteDir(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		log.Fatalf("fail to remove artifact '%s': %v", dir, err)
	}
}

func linkInputArt(artsDir, artName string) {
	err := os.Symlink(path.Join(InputArtifactsDir, artName),
		path.Join(artsDir, artName))

	if err != nil {
		log.Fatalf("error creating Symlink: %v", err)
	}
}

func LinkInputArts(artsDir string, artNames ...string) {
	logrus.Infof("LinkInputArts(artsDir='%s', artNames...='%v') called ", artsDir, artNames)
	for _, art := range artNames {
		linkInputArt(artsDir, art)
	}
}

func NewArtsDir(dirPrefix string) string {
	os.MkdirAll(tmpArtsDir, 0755)
	dir, err := ioutil.TempDir(tmpArtsDir, dirPrefix+"_")
	logrus.Infof("artsDir='%s'", dir)
	if err != nil {
		log.Fatalf("Error making TempDir for arts: %v\n", err)
	} else {
		logrus.Infof("Temp arts dir created: %s\n", dir)
	}
	return dir
}
