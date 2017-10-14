/*
Copyright 2017 The Kubernetes Authors.

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

	flag "github.com/spf13/pflag"
)

// Args contains all of the (parsed) command line arguments for bootstrap
// NOTE: Repo should be further parsed by ParseRepos
type Args struct {
	Root           string
	Timeout        int32
	Repo           []string
	Bare           bool
	Job            string
	Upload         string
	ServiceAccount string
	SSH            string
	GitCache       string
	Clean          bool
	// Note: these are the args after `--` terminates the other args
	// IE `bootstrap --job=foo -- --bar=baz` -> JobArgs == []string{"--bar=baz"}
	JobArgs []string
}

// ParseArgs parses the command line to an Args instance
// arguments should be generally be os.Args[1:]
func ParseArgs(arguments []string) (*Args, error) {
	args := &Args{}
	flags := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	// used to mimic required=true
	requiredFlags := []string{"job"}

	// add all of the args from parse_args in jenkins/bootstrap.py
	flags.StringVar(&args.Root, "root", ".", "Root dir to work with")

	// NOTE: jenkins/bootstrap.py technically used a float for this arg
	// when parsing but only ever used the arg as an integer number
	// of timeout minutes.
	// int32 makes more sense and will let use time.Minute * timeout
	flags.Int32Var(&args.Timeout, "timeout", 0, "Timeout in minutes if set")

	flags.StringArrayVar(&args.Repo, "repo", []string{},
		"Fetch the specified repositories, with the first one considered primary")

	flags.BoolVar(&args.Bare, "bare", false, "Do not check out a repository")
	flags.Lookup("bare").NoOptDefVal = "true" // allows using --bare

	// NOTE: this arg is required (set above)
	flags.StringVar(&args.Job, "job", "", "Name of the job to run")

	flags.StringVar(&args.Upload, "upload", "",
		"Upload results here if set, requires --service-account")
	flags.StringVar(&args.ServiceAccount, "service-account", "",
		"Activate and use path/to/service-account.json if set.")
	flags.StringVar(&args.SSH, "ssh", "",
		"Use the ssh key to fetch the repository instead of https if set.")
	flags.StringVar(&args.GitCache, "git-cache", "", "Location of the git cache.")

	flags.BoolVar(&args.Clean, "clean", false, "Clean the git repo before running tests.")
	flags.Lookup("clean").NoOptDefVal = "true" // allows using --clean

	// parse flags
	// NOTE: this stops parsing at `--`, after which we grab arguments as
	// JobArgs below.
	err := flags.Parse(arguments)
	if err != nil {
		return nil, err
	}
	for i, arg := range arguments {
		if arg == "--" {
			args.JobArgs = arguments[i+1:]
			break
		}
	}

	// check that required args were set
	for _, arg := range requiredFlags {
		if flag := flags.Lookup(arg); !flag.Changed {
			err = fmt.Errorf("flag '--%s' is required but was not set", flag.Name)
			return nil, err
		}
	}

	// validate args
	if args.Bare == (len(args.Repo) != 0) {
		err = fmt.Errorf("expected --repo xor --bare, Got: --repo=%v, --bare=%v", args.Repo, args.Bare)
		return nil, err
	}
	if args.Job == "" {
		return nil, fmt.Errorf("--job=\"\" is not valid, please supply a job name")
	}

	return args, nil
}
