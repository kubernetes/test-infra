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

package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/test-infra/kubetest2/pkg/app/shim"
	"k8s.io/test-infra/kubetest2/pkg/app/testers"
	"k8s.io/test-infra/kubetest2/pkg/types"
)

// Run instantiates and executes the kubetest2 cobra command, returning the result
func Run(deployerName, deployerUsage string, newDeployer types.NewDeployer) error {
	return NewCommand(deployerName, deployerUsage, newDeployer).Execute()
}

// NewCommand returns a new cobra.Command for kubetest2
func NewCommand(deployerName, deployerUsage string, newDeployer types.NewDeployer) *cobra.Command {
	cmd := &cobra.Command{
		Use: fmt.Sprintf("%s %s", shim.BinaryName, deployerName),
		// we defer showing usage, so that we can include deployer and test
		// specific usage in RealMain(...)
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runE(cmd, args, deployerName, deployerUsage, newDeployer)
		},
	}
	// we implement custom flag parsing below
	cmd.DisableFlagParsing = true
	return cmd
}

// runE implements the custom CLI logic
func runE(
	cmd *cobra.Command, args []string,
	deployerName, deployerUsage string, newDeployer types.NewDeployer,
) error {
	// setup the options struct & flags
	opts := newOptions(deployerName, deployerUsage)

	// go ahead show help if there are zero arguments
	// since there are none to parse, we cannot identify the desired tester
	if len(args) < 1 {
		cmd.Print(opts.usage())
		return nil
	}

	// parse the arguments
	deployerArgs, testerArgs, err := parseArgs(opts, args)
	if err != nil {
		cmd.Printf("Error: could not parse arguments: %v\n", err)
		return err
	}

	// now that we've parsed flags we can look up the tester
	var newTester types.NewTester
	if opts.test != "" {
		n, usage, ok := testers.Get(opts.test)
		// now that we know which tester, plumb the help info
		opts.testerUsage = usage
		newTester = n
		// fail if the named tester does not exist
		if !ok {
			// TODO(bentheelder): inform the user which testers exist
			return errors.Errorf("no such tester: %#v", opts.test)
		}
	}

	// instantiate the deployer
	deployer, err := newDeployer(opts, deployerArgs)
	if err != nil {
		if usage, ok := err.(types.IncorrectUsage); ok {
			cmd.Print(usage.HelpText())
			cmd.Print("\n\n")
			cmd.Print(opts.usage())
		}
		return err
	}

	// instantiate the tester if testing was specified
	// NOTE: we need to do this before building etc, so we can fail fast on
	// invalid options to the tester
	var tester types.Tester
	if newTester != nil {
		tester, err = newTester(opts, testerArgs, deployer)
		if err != nil {
			if usage, ok := err.(types.IncorrectUsage); ok {
				cmd.Print(usage.HelpText())
				cmd.Print("\n\n")
				cmd.Print(opts.usage())
			}
			return err
		}
	}

	// run RealMain, which contains all of the logic beyond the CLI boilerplate
	return RealMain(opts, deployer, tester)
}

// parseArgs attaches all kubetest2 first class flags to opts, parses the args,
// and returns the deployer args, test args, and error if any
func parseArgs(opt *options, args []string) ([]string, []string, error) {
	// first split into args and test args
	testArgs := []string{}
	for i := range args {
		if args[i] == "--" {
			if i+1 < len(args) {
				testArgs = args[i+1:]
			}
			args = args[:i]
			break
		}
	}

	// finally, parse flags
	err := opt.flags.Parse(args)

	// NOTE: we still return args in all cases so that the deployer and
	// tester have a chance to construct help output
	return opt.flags.Args(), testArgs, err
}

// the default is $ARTIFACTS if set, otherwise ./_artifacts
func defaultArtifactsDir() string {
	path, set := os.LookupEnv("ARTIFACTS")
	if set {
		return path
	}
	return "./_artifacts"
}

// options holds flag values and implements deployer.Options
type options struct {
	// cli info for usage()
	name          string
	deployerUsage string
	testerUsage   string
	flags         *pflag.FlagSet
	// options exposed via Options interface
	help      bool
	build     bool
	up        bool
	down      bool
	test      string
	artifacts string
}

func newOptions(name, deployerUsage string) *options {
	opt := &options{
		name:          name,
		deployerUsage: deployerUsage,
	}

	flags := pflag.NewFlagSet(name, pflag.ContinueOnError)
	// unknown flags are forwarded to the deployer as arguments
	flags.ParseErrorsWhitelist.UnknownFlags = true

	// register all first class kubetest2 flags
	flags.BoolVarP(&opt.help, "help", "h", false, "display help")
	flags.BoolVar(&opt.build, "build", false, "build kubernetes")
	flags.BoolVar(&opt.up, "up", false, "provision the test cluster")
	flags.BoolVar(&opt.down, "down", false, "tear down the test cluster")
	flags.StringVar(&opt.test, "test", "", "test type to run, if unset no tests will run")
	flags.StringVar(&opt.artifacts, "artifacts", defaultArtifactsDir(), `directory to put artifacts, defaulting to "${ARTIFACTS:-./_artifacts}"`)

	opt.flags = flags
	return opt
}

func (o *options) usage() string {
	u := fmt.Sprintf(
		strings.TrimPrefix(`
Usage:
  kubetest2 %s [Flags] [DeployerArgs] -- [TesterArgs]

Flags:
%s
DeployerArgs(%s):
%s
`, "\n"),
		o.name,
		o.flags.FlagUsages(),
		o.name,
		o.deployerUsage,
	)
	// add tester info if we selected a tester and have it
	if o.testerUsage != "" {
		u += fmt.Sprintf(
			strings.TrimPrefix(`
TesterArgs(%s):
%s
`, "\n"),
			o.test,
			o.testerUsage,
		)
	}
	return u
}

// assert that options implements deployer options
var _ types.Options = &options{}

func (o *options) HelpRequested() bool {
	return o.help
}

func (o *options) ShouldBuild() bool {
	return o.build
}

func (o *options) ShouldUp() bool {
	return o.up
}

func (o *options) ShouldDown() bool {
	return o.down
}

func (o *options) ShouldTest() bool {
	return o.test != ""
}

func (o *options) ArtifactsDir() string {
	return o.artifacts
}
