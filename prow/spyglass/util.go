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

package spyglass

import (
	"bufio"
	"bytes"
	"strings"
)

// LastNLines reads the last n lines from a file in GCS
func LastNLines(a Artifact, n int64) string {
	chunkSize := int64(1e6) //1MB
	toRead := chunkSize + 1 // Add 1 for exclusive upper bound read range
	chunks := int64(1)
	contents := []string{}
	artifactSize := a.Size()
	offset := artifactSize - chunks*chunkSize
	for int64(len(contents)) < n && offset != 0 {
		chunkContents := []string{}
		offset = artifactSize - chunks*chunkSize
		if offset < 0 {
			toRead = offset + chunkSize + 1
			offset = 0
		}
		bytesRead := make([]byte, toRead)
		a.ReadAt(bytesRead, offset)
		bytesRead = bytes.Trim(bytesRead, "\x00")
		scanner := bufio.NewScanner(bytes.NewReader(bytesRead))
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			chunkContents = append(chunkContents, scanner.Text())
		}
		contents = append(contents, chunkContents...)
		chunks += 1
	}
	l := int64(len(contents))
	if l < n {
		return strings.Join(contents, "\n")
	}
	return strings.Join(contents[l-n:], "\n")

}
