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

// config-forker CLI forks Prow job configurations for new release branches.
package main

import (
	"context"
	"flag"
	"log"

	forker "k8s.io/test-infra/releng/config-forker/pkg"
)

func main() {
	opts := forker.Options{}

	flag.StringVar(&opts.JobConfig, "job-config", "", "Path to the job config")
	flag.StringVar(&opts.OutputPath, "output", "", "Path to the output yaml. If not specified, just validate.")
	flag.StringVar(&opts.Version, "version", "", "Version number to generate jobs for")
	flag.StringVar(&opts.GoVersion, "go-version", "", "Current go version in use")
	flag.Parse()

	opts.ImageResolver = forker.NewRegistryResolver(nil)

	if err := forker.Run(context.Background(), opts); err != nil {
		log.Fatalln(err)
	}
}
