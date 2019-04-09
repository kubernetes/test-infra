/*
Copyright 2019 The Kubernetes Authors.

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

package main

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"time"

	"k8s.io/klog"
)

// These methods are only used for local machine benchmarking, so they may be poor optimized or violate good style
const testFilePath = "c:\\temp\\kube-apiserver.log"

func setupForLocal() {
	downloadToFiles()
}

// We assume read from file is fast - according to 'testRead' it takes ~1s on uncompressed file
func runBenchmarks() {
	testDecompress()
	testDownload()
	testParse()
	testFileRead()
}

func testDecompress() {
	defer timeTrack(time.Now(), "decompress")
	r := readFromLocalFile(testFilePath + ".gz")
	r, err := decompress(r)
	io.Copy(ioutil.Discard, r)
	handle(err)
}

func testDownload() {
	defer timeTrack(time.Now(), "download")

	r, err := download(testLogPath)
	handle(err)
	io.Copy(ioutil.Discard, r)
}

func testParse() {
	defer timeTrack(time.Now(), "parse")
	regex, _ := regexp.Compile(testTargetSubstring)
	reader := bufio.NewReader(readFromLocalFile(testFilePath))
	parsed, _ := processLines(reader, regex)
	if len(parsed) == 0 {
	}
}

func testFileRead() {
	defer timeTrack(time.Now(), "read")
	r := readFromLocalFile(testFilePath)
	io.Copy(ioutil.Discard, r)
}

func downloadToFiles() {
	reader, err := download(testLogPath)
	handle(err)
	bts, err := ioutil.ReadAll(reader)
	handle(err)

	ioutil.WriteFile(testFilePath+".gz", bts, 0644)
	handle(err)

	r, err := decompress(bytes.NewReader(bts))
	handle(err)
	fullFile, err := os.OpenFile(testFilePath, os.O_RDWR|os.O_CREATE, 0644)
	handle(err)
	_, err = io.Copy(fullFile, r)
	handle(err)
}

func readFromLocalFile(filename string) io.Reader {
	bts, err := ioutil.ReadFile(filename)
	handle(err)
	return bytes.NewReader(bts)
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	klog.Infof("%s took %s", name, elapsed)
}

func handle(err error) {
	if err == nil {
		return
	}
	klog.Fatal(err)
}
