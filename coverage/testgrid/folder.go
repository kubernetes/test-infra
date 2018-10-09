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

package testgrid

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// get a list a sub-directories that contains source code. The list will be shown on Testgrid
func getDirs(coverageStdout string) (res []string) {
	file, err := os.Open(coverageStdout)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		res = append(res, strings.Split(scanner.Text(), "\t")[1])
	}
	return
}
