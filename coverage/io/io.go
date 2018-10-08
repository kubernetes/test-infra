/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package io

import (
	"fmt"
	"os"
	"path"

	"github.com/sirupsen/logrus"
)

// CreateMarker produces empty file as marker
func CreateMarker(dir, fileName string) error {
	err := Write(nil, dir, fileName)
	logrus.Infof("Created marker file '%s'", fileName)
	return err
}

// Write writes the content of the string to a file in the directory
func Write(content *string, destinationDir, fileName string) error {
	filePath := path.Join(destinationDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error writing file: %v", err)
	}

	logrus.Infof("Created file:%s", filePath)
	if content == nil {
		logrus.Infof("No content to be written to file '%s'", fileName)
	} else {
		_, err = fmt.Fprint(file, *content)
		if err != nil {
			return fmt.Errorf("cannot print to file: %v", err)
		}
	}
	return file.Close()
}
