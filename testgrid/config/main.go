/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"os"
	"fmt"

	"k8s.io/test-infra/testgrid/config/yaml2proto"
)

//
// usage: config <input/path/to/yaml> <output/path/to/proto>
//

func main(){
	args := os.Args[1:]

	if len(args) != 2 {
		fmt.Printf("Wrong Arguments - usage: yaml2proto <input/path/to/yaml> <output/path/to/proto>\n")
	}

	err := yaml2proto.Yaml2Proto(args[0],args[1])
	if err != nil {
		fmt.Printf("Yaml2Proto Error : %v\n", err)
	}
}

