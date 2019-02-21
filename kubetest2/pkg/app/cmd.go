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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/test-infra/kubetest2/pkg/app/shim"
	"k8s.io/test-infra/kubetest2/pkg/app/testers"
	"k8s.io/test-infra/kubetest2/pkg/types"
)

// Run instantiates and executes the kubetest2 cobra command, returning the result
func Run(deployerName string, newDeployer types.NewDeployer) error {
	return NewCommand(deployerName, newDeployer).Execute()
}

// NewCommand returns a new cobra.Command for kubetest2
func NewCommand(deployerName string, newDeployer types.NewDeployer) *cobra.Command {
	opt := &options{}
	cmd := &cobra.Command{
		Use: fmt.Sprintf("%s %s", shim.BinaryName, deployerName),
		// we defer showing usage, so that we can include deployer and test
		// specific usage in RealMain(...)
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runE(cmd, args, opt, deployerName, newDeployer)
		},
	}

	// we implement custom flag parsing below
	cmd.DisableFlagParsing = true

	return cmd
}

// runE implements the custom CLI logic
func runE(
	cmd *cobra.Command, args []string,
	opts *options, deployerName string, newDeployer types.NewDeployer,
) error {
	// show help if there are zero arguments
	if len(args) < 1 {
		return cmd.Help()
	}

	deployerArgs, testerArgs, err := parseArgs(deployerName, opts, args)
	if err != nil {
		cmd.Printf("Error: could not parse arguments: %v\n", err)
		return err
	}

	// instantiate the deployer
	deployer, err := newDeployer(opts, deployerArgs)
	if err != nil {
		if usage, ok := err.(types.IncorrectUsage); ok {
			cmd.Print(usage.HelpText())
		}
		return err
	}

	// instantiate the tester if testing was specified
	// NOTE: we need to do this before building etc, so we can fail fast on
	// invalid options to the tester
	var tester types.Tester
	if opts.test != "" {
		newTester, ok := testers.Get(opts.test)
		if !ok {
			// TODO(bentheelder): inform the user which testers exist
			return errors.Errorf("no such tester: %#v", opts.test)
		}
		tester, err = newTester(opts, testerArgs, deployer)
		if err != nil {
			if usage, ok := err.(types.IncorrectUsage); ok {
				cmd.Print(usage.HelpText())
			}
			return err
		}
	}

	// run RealMain, which contains all of the logic beyond the CLI boilerplate
	return RealMain(opts, deployer, tester)
}

// parseArgs attaches all kubetest2 first class flags to opts, parses the args,
// and returns the deployer args, test args, and error if any
func parseArgs(name string, opt *options, args []string) ([]string, []string, error) {
	// first split into args and test args
	testArgs := []string{}
	for i := range args {
		if args[i] == "--" {
			if i+1 < len(args) {
				testArgs = args[i+1:]
			}
			args = args[:i]
		}
	}

	flags := pflag.NewFlagSet(name, pflag.ContinueOnError)
	// unknown flags are forwarded to the deployer as arguments
	flags.ParseErrorsWhitelist.UnknownFlags = true

	// register all first class kubetest2 flags
	flags.BoolVarP(&opt.help, "help", "h", false, "")
	flags.BoolVar(&opt.build, "build", false, "build kubernetes")
	flags.BoolVar(&opt.up, "up", false, "provision the test cluster")
	flags.BoolVar(&opt.down, "down", false, "tear down the test cluster")
	flags.StringVar(&opt.test, "test", "", "test type to run, if unset no tests will run")

	// finally, parse flags
	if err := flags.Parse(args); err != nil {
		return nil, nil, err
	}
	return flags.Args(), testArgs, nil
}

// options holds flag values and implements deployer.Options
type options struct {
	help  bool
	build bool
	up    bool
	down  bool
	test  string
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
