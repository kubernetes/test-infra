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
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
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
		Use:   "html [coverage...]",
		Short: "Emits an HTML file to browse coverage files.",
		Long: `Produces a self-contained HTML file that enables browsing the provided
coverage files by directory. The resulting file can be distributed alone to
produce the same rendering (but does currently require gstatic.com to be
accessible).

If multiple files are provided, they will all be
shown in the generated HTML file, with the columns in the same order the files
were listed. When there are multiples columns, each column will have an arrow
indicating the change from the column immediately to its right.`,
		Run: func(cmd *cobra.Command, args []string) {
			run(flags, cmd, args)
		},
	}
	cmd.Flags().StringVarP(&flags.OutputFile, "output", "o", "-", "output file")
	return cmd
}

type coverageFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func run(flags *flags, cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		fmt.Println("Expected at least one coverage file.")
		cmd.Usage()
		os.Exit(2)
	}

	// This path assumes we're being run using bazel.
	resourceDir := "gopherage/cmd/html/static"
	if _, err := os.Stat(resourceDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Resource directory does not exist.")
		os.Exit(1)
	}

	tpl, err := template.ParseFiles(filepath.Join(resourceDir, "browser.html"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't read the HTML template: %v.", err)
		os.Exit(1)
	}
	script, err := ioutil.ReadFile(filepath.Join(resourceDir, "browser_bundle.es6.js"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't read JavaScript: %v.", err)
		os.Exit(1)
	}

	// If we're under bazel, move into BUILD_WORKING_DIRECTORY so that manual
	// invocations of bazel run are less confusing.
	if wd, ok := os.LookupEnv("BUILD_WORKING_DIRECTORY"); ok {
		if err := os.Chdir(wd); err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't chdir into expected working directory.")
			os.Exit(1)
		}
	}

	var coverageFiles []coverageFile
	for _, arg := range args {
		var content []byte
		var err error
		if arg == "-" {
			content, err = ioutil.ReadAll(os.Stdin)
		} else {
			content, err = ioutil.ReadFile(arg)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't read coverage file: %v.", err)
			os.Exit(1)
		}
		coverageFiles = append(coverageFiles, coverageFile{Path: arg, Content: string(content)})
	}

	outputPath := flags.OutputFile
	var output io.Writer
	if outputPath == "-" {
		output = os.Stdout
	} else {
		f, err := os.Create(outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't open output file: %v.", err)
			os.Exit(1)
		}
		defer f.Close()
		output = f
	}

	err = tpl.Execute(output, struct {
		Script   template.JS
		Coverage []coverageFile
	}{template.JS(script), coverageFiles})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't write output file: %v.", err)
	}
}
