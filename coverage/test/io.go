package test

import (
	"io/ioutil"
	"log"
	"os"
	"path"
)

func DeleteDir(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		log.Fatalf("fail to remove artifact '%s': %v", dir, err)
	}
}

func MkdirAll(path string) {
	if err := os.MkdirAll(path, 0755); err != nil {
		log.Fatalf("Failed os.MkdirAll(path='%s', 0755); err='%v'", path, err)
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
	log.Printf("LinkInputArts(artsDir='%s', artNames...='%v') called ", artsDir, artNames)
	for _, art := range artNames {
		linkInputArt(artsDir, art)
	}
}

func NewArtsDir(dirPrefix string) string {
	MkdirAll(tmpArtsDir)
	dir, err := ioutil.TempDir(tmpArtsDir, dirPrefix+"_")
	log.Printf("artsDir='%s'", dir)
	if err != nil {
		log.Fatalf("Error making TempDir for arts: %v\n", err)
	} else {
		log.Printf("Temp arts dir created: %s\n", dir)
	}
	return dir
}
