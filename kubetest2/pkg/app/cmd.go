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
	// setup the options struct & flags, etc.
	opts := &options{}
	flags := pflag.NewFlagSet(deployerName, pflag.ContinueOnError)
	opts.bindFlags(flags)
	usage := newUsage(opts, flags, deployerName, deployerUsage)
	// NOTE: unknown flags are forwarded to the deployer as arguments
	flags.ParseErrorsWhitelist.UnknownFlags = true

	// go ahead show help if there are zero arguments
	// since there are none to parse, we cannot identify the desired tester
	if len(args) < 1 {
		cmd.Print(usage.String())
		return nil
	}

	// parse the arguments
	deployerArgs, testerArgs, err := parseArgs(flags, args)
	if err != nil {
		cmd.Printf("Error: could not parse arguments: %v\n", err)
		return err
	}

	// now that we've parsed flags we can look up the tester
	var newTester types.NewTester
	if usage.options.test != "" {
		n, testerUsage, ok := testers.Get(opts.test)
		// now that we know which tester, plumb the help info
		usage.testerUsage = testerUsage
		newTester = n
		// fail if the named tester does not exist
		if !ok {
			// TODO(bentheelder): inform the user which testers exist
			return errors.Errorf("no such tester: %#v", opts.test)
		}
	}

	// instantiate the deployer
	deployer, err := newDeployer(opts, flagsForDeployer(deployerName), deployerArgs)
	if err != nil {
		if v, ok := err.(types.IncorrectUsage); ok {
			cmd.Print(v.HelpText())
			cmd.Print("\n\n")
			cmd.Print(usage.String())
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
			if v, ok := err.(types.IncorrectUsage); ok {
				cmd.Print(v.HelpText())
				cmd.Print("\n\n")
				cmd.Print(usage.String())
			}
			return err
		}
	}

	// run RealMain, which contains all of the logic beyond the CLI boilerplate
	return RealMain(opts, deployer, tester)
}

// the default is $ARTIFACTS if set, otherwise ./_artifacts
func defaultArtifactsDir() string {
	path, set := os.LookupEnv("ARTIFACTS")
	if set {
		return path
	}
	return "./_artifacts"
}

// flagsForDeployer creates a copy of all kubetest2 flags for usage in deployer
// parsing, these flags will be passed to the deployer
// All flags will be marked hidden so they don't show up in the deployer usage
func flagsForDeployer(name string) *pflag.FlagSet {
	// create a generic flagset and bind all of the flags
	flags := pflag.NewFlagSet(name, pflag.ContinueOnError)
	opt := &options{}
	opt.bindFlags(flags)
	// mark all flags as hidden
	flags.VisitAll(func(flag *pflag.Flag) {
		flag.Hidden = true
	})
	return flags
}

// parseArgs attaches all kubetest2 first class flags to opts, parses the args,
// and returns the kubetest2 args, test args, and error if any
func parseArgs(flags *pflag.FlagSet, args []string) ([]string, []string, error) {
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
	err := flags.Parse(args)

	// NOTE: we still return args in all cases so that the deployer and
	// tester have a chance to construct help output
	return args, testArgs, err
}

// options holds flag values and implements deployer.Options
type options struct {
	help      bool
	build     bool
	up        bool
	down      bool
	test      string
	artifacts string
}

// bindFlags registers all first class kubetest2 flags
func (o *options) bindFlags(flags *pflag.FlagSet) {
	flags.BoolVarP(&o.help, "help", "h", false, "display help")
	flags.BoolVar(&o.build, "build", false, "build kubernetes")
	flags.BoolVar(&o.up, "up", false, "provision the test cluster")
	flags.BoolVar(&o.down, "down", false, "tear down the test cluster")
	flags.StringVar(&o.test, "test", "", "test type to run, if unset no tests will run")
	flags.StringVar(&o.artifacts, "artifacts", defaultArtifactsDir(), `directory to put artifacts, defaulting to "${ARTIFACTS:-./_artifacts}"`)
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

// helper for computing the usage string & tracking usage related metadata
type usage struct {
	// cli info for String()
	name          string
	deployerUsage string
	testerUsage   string
	// flags and flag variables
	flags   *pflag.FlagSet
	options *options
}

func newUsage(opts *options, flags *pflag.FlagSet, name, deployerUsage string) *usage {
	return &usage{
		name:          name,
		deployerUsage: deployerUsage,
		options:       opts,
		flags:         flags,
	}
}

func (u *usage) String() string {
	s := fmt.Sprintf(
		strings.TrimPrefix(`
Usage:
  kubetest2 %s [Flags] [DeployerArgs] -- [TesterArgs]

Flags:
%s
DeployerArgs(%s):
%s
`, "\n"),
		u.name,
		u.flags.FlagUsages(),
		u.name,
		u.deployerUsage,
	)
	// add tester info if we selected a tester and have it
	if u.testerUsage != "" {
		s += fmt.Sprintf(
			strings.TrimPrefix(`
TesterArgs(%s):
%s
`, "\n"),
			u.options.test,
			u.testerUsage,
		)
	}
	return s
}
