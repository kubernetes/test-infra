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

// config-rotator CLI rotates Prow job configurations through stability tiers.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	rotator "k8s.io/test-infra/releng/config-rotator/pkg"
)

func cdToRootDir() error {
	if bazelWorkspace := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); bazelWorkspace != "" {
		if err := os.Chdir(bazelWorkspace); err != nil {
			return fmt.Errorf("failed to chdir to bazel workspace (%s): %w", bazelWorkspace, err)
		}
	}

	return nil
}

func main() {
	if err := cdToRootDir(); err != nil {
		log.Fatalln(err)
	}

	opts := rotator.Options{}
	flag.StringVar(&opts.ConfigFile, "config-file", "", "Path to the job config")
	flag.StringVar(&opts.OldVersion, "old", "", "Old version (beta, stable1, or stable2)")
	flag.StringVar(&opts.NewVersion, "new", "", "New version (stable1, stable2, or stable3)")
	flag.Parse()

	if err := rotator.Run(opts); err != nil {
		log.Fatalln(err)
	}
}
