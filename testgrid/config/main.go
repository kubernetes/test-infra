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
	"fmt"
	"io/ioutil"
	"os"

	"k8s.io/test-infra/testgrid/config/yaml2proto"
)

//
// usage: config <input/path/to/yaml> <output/path/to/proto>
//

func main() {
	args := os.Args[1:]

	if len(args) != 2 {
		fmt.Printf("Wrong Arguments - usage: yaml2proto <input/path/to/yaml> <output/path/to/proto>\n")
		os.Exit(-1)
	}

	yamlData, err := ioutil.ReadFile(args[0])
	if err != nil {
		fmt.Printf("IO Error : Cannot Read File %v\n", args[0])
		os.Exit(-1)
	}

	protobufData, err := yaml2proto.Yaml2Proto(yamlData)
	if err != nil {
		fmt.Printf("Yaml2Proto Error : %v\n", err)
		os.Exit(-1)
	}

	err = ioutil.WriteFile(args[1], protobufData, 0777)
	if err != nil {
		fmt.Printf("IO Error : Cannot Write File %v\n", args[1])
		os.Exit(-1)
	}
}
