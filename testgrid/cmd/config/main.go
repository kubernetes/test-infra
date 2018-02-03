/*
Copyright 2016 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

var (
	yamlPaths  = flag.String("yaml", "", "comma-separated list of input YAML files")
	printText  = flag.Bool("print-text", false, "print generated proto in text format to stdout")
	outputPath = flag.String("output", "", "output path to save generated protobuf data")
)

func errExit(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	os.Exit(1)
}

func main() {
	flag.Parse()

	yamlFiles := strings.Split(*yamlPaths, ",")
	if len(yamlFiles) == 0 || yamlFiles[0] == "" {
		errExit("Must specify one or more YAML files with --yaml\n")
	}
	if !*printText && *outputPath == "" {
		errExit("Must set --print-text or --output\n")
	}
	if *printText && *outputPath != "" {
		errExit("Cannot set both --print-text and --output\n")
	}

	var c Config
	for _, file := range yamlFiles {
		b, err := ioutil.ReadFile(file)
		if err != nil {
			errExit("IO Error : Cannot Read File %s : %v\n", file, err)
		}
		if err = c.Update(b); err != nil {
			errExit("Error parsing file %s : %v\n", file, err)
		}
	}

	if *printText {
		if err := c.MarshalText(os.Stdout); err != nil {
			errExit("err printing proto: %v", err)
		}
	} else {
		b, err := c.MarshalBytes()
		if err != nil {
			errExit("err encoding proto: %v", err)
		}
		if err = ioutil.WriteFile(*outputPath, b, 0644); err != nil {
			errExit("IO Error : Cannot Write File %v\n", outputPath)
		}
	}
}
