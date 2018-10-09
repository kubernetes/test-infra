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

package githubUtil

import (
	"strings"

	"github.com/sirupsen/logrus"
)

// FilePathProfileToGithub converts filepath from profile format to github format
// (github.com/$REPO_OWNER/$REPO_NAME/pkg/... -> pkg/...)
func FilePathProfileToGithub(filePath string) string {
	slice := strings.SplitN(filePath, "/", 4)
	if len(slice) < 4 {
		logrus.Infof("FilePath string cannot be splitted into 4 parts: [sep=%s] %s; "+
			"Original string is returned\n", "/", filePath)
		return filePath
	}
	return slice[3]
}
