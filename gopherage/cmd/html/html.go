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

package html

import (
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type flags struct {
	OutputFile string
}

// MakeCommand returns a `diff` command.
func MakeCommand() *cobra.Command {
	flags := &flags{}
	cmd := &cobra.Command{
		Use:   "html [coverage]",
		Short: "Emits an HTML file to browse a coverage file.",
		Run: func(cmd *cobra.Command, args []string) {
			run(flags, cmd, args)
		},
	}
	cmd.Flags().StringVar(&flags.OutputFile, "o", "-", "output file")
	return cmd
}

func run(flags *flags, cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		log.Fatalln("Usage: gopherage html [coverage...]")
	}

	// This path assumes we're being run using bazel.
	resourceDir := "gopherage/cmd/html/static"
	if _, err := os.Stat(resourceDir); os.IsNotExist(err) {
		log.Fatalln("Resource directory does not exist.")
	}

	tpl, err := template.ParseFiles(filepath.Join(resourceDir, "browser.html"))
	if err != nil {
		log.Fatalf("Couldn't read the HTML template: %v", err)
	}
	script, err := ioutil.ReadFile(filepath.Join(resourceDir, "browser_bundle.es6.js"))
	if err != nil {
		log.Fatalf("Couldn't read JavaScript: %v", err)
	}

	// If we're under bazel, move into BUILD_WORKING_DIRECTORY so that manual
	// invocations of bazel run are less confusing.
	if wd, ok := os.LookupEnv("BUILD_WORKING_DIRECTORY"); ok {
		if err := os.Chdir(wd); err != nil {
			log.Fatalf("Couldn't chdir into expected working directory.")
		}
	}

	coverageFiles := make(map[string]string, len(args))
	for _, arg := range args {
		content, err := ioutil.ReadFile(arg)
		if err != nil {
			log.Fatalf("Couldn't read coverage file: %v", err)
		}
		coverageFiles[arg] = string(content)
	}

	outputPath := flags.OutputFile
	var output io.Writer
	if outputPath == "-" {
		output = os.Stdout
	} else {
		f, err := os.Create(outputPath)
		if err != nil {
			log.Fatalf("Couldn't open output file: %v", err)
		}
		defer f.Close()
		output = f
	}

	tpl.Execute(output, struct {
		Script   template.JS
		Coverage map[string]string
	}{template.JS(script), coverageFiles})
}
