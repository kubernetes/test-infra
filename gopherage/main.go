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

package main

import (
	"log"

	"github.com/spf13/cobra"
	"k8s.io/test-infra/gopherage/cmd/aggregate"
	"k8s.io/test-infra/gopherage/cmd/diff"
	"k8s.io/test-infra/gopherage/cmd/filter"
	"k8s.io/test-infra/gopherage/cmd/html"
	"k8s.io/test-infra/gopherage/cmd/junit"
	"k8s.io/test-infra/gopherage/cmd/merge"
)

var rootCommand = &cobra.Command{
	Use:   "gopherage",
	Short: "gopherage is a tool for manipulating Go coverage files.",
}

func run() error {
	rootCommand.AddCommand(aggregate.MakeCommand())
	rootCommand.AddCommand(diff.MakeCommand())
	rootCommand.AddCommand(filter.MakeCommand())
	rootCommand.AddCommand(html.MakeCommand())
	rootCommand.AddCommand(junit.MakeCommand())
	rootCommand.AddCommand(merge.MakeCommand())
	return rootCommand.Execute()
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
