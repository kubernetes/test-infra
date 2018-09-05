package main

import (
	"github.com/spf13/cobra"
	"k8s.io/test-infra/gopherage/cmd/diff"
	"k8s.io/test-infra/gopherage/cmd/merge"
	"log"
)

var rootCommand = &cobra.Command{
	Use:   "gopherage",
	Short: "gopherage is a tool for manipulating Go coverage files.",
}

func run() error {
	rootCommand.AddCommand(diff.MakeCommand())
	rootCommand.AddCommand(merge.MakeCommand())
	return rootCommand.Execute()
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
