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
	"os"

	docker "github.com/docker/engine-api/client"
	"k8s.io/contrib/docker-micro-benchmark/helpers"
)

func main() {
	usage := func() {
		fmt.Printf("Usage: %s -[o|c|i|r]\n", os.Args[0])
	}
	if len(os.Args) != 2 {
		usage()
		return
	}
	client, _ := docker.NewClient(endpoint, apiVersion, nil, nil)
	d := helpers.NewDockerHelper(client)
	d.PullTestImage()
	switch os.Args[1] {
	case "-o":
		benchmarkContainerStart(d)
	case "-c":
		benchmarkVariesContainerNumber(d)
	case "-i":
		benchmarkVariesInterval(d)
	case "-r":
		benchmarkVariesRoutineNumber(d)
	default:
		usage()
	}
}
