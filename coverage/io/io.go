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
