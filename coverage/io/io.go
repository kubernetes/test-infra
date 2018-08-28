package io

import (
	"fmt"
	"os"
	"path"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/coverage/logUtil"
)

// CreateMarker produces empty file as marker
func CreateMarker(dir, fileName string) {
	Write(nil, dir, fileName)
	logrus.Infof("Created marker file '%s'\n", fileName)
}

// Write writes the content of the string to a file in the directory
func Write(content *string, destinationDir, fileName string) {
	filePath := path.Join(destinationDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		logUtil.LogFatalf("Error writing file: %v", err)
	} else {
		logrus.Infof("Created file:%s", filePath)
		if content == nil {
			logrus.Infof("No content to be written to file '%s'", fileName)
		} else {
			fmt.Fprint(file, *content)
		}
	}
	defer file.Close()
}

//MkdirAll makes directory on disk. Recursively adds parents directory if not exist.
func MkdirAll(path string) {
	logrus.Infof("Making directory (MkdirAll): path=%s", path)
	if err := os.MkdirAll(path, 0755); err != nil {
		logrus.Fatalf("Failed os.MkdirAll(path='%s', 0755); err='%v'", path, err)
	} else {
		logrus.Infof("artifacts dir (path=%s) created successfully\n", path)
	}
}

//FileOrDirExists checks whether a file or dir on disk exist
func FileOrDirExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			cwd, _ := os.Getwd()
			logrus.Infof("file or dir not found: %s; cwd=%s", path, cwd)
			return false
		}
		logrus.Fatalf("File stats (path=%s) err: %v", path, err)
	}
	return true
}
