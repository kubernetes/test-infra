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
func Run(deployerName string, newDeployer types.NewDeployer) error {
	return NewCommand(deployerName, newDeployer).Execute()
}

// NewCommand returns a new cobra.Command for kubetest2
func NewCommand(deployerName string, newDeployer types.NewDeployer) *cobra.Command {
	cmd := &cobra.Command{
		Use: fmt.Sprintf("%s %s", shim.BinaryName, deployerName),
		// we defer showing usage, so that we can include deployer and test
		// specific usage in RealMain(...)
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runE(cmd, args, deployerName, newDeployer)
		},
	}
	// we implement custom flag parsing below
	cmd.DisableFlagParsing = true
	return cmd
}

// runE implements the custom CLI logic
func runE(
	cmd *cobra.Command, args []string,
	deployerName string, newDeployer types.NewDeployer,
) error {
	// setup the options struct & flags, etc.
	opts := &options{}
	kubetest2Flags := pflag.NewFlagSet(deployerName, pflag.ContinueOnError)
	opts.bindFlags(kubetest2Flags)

	// NOTE: unknown flags are forwarded to the deployer as arguments
	kubetest2Flags.ParseErrorsWhitelist.UnknownFlags = true

	// parse arguments, splitting out test args (after the `--`)
	deployerArgs, testerArgs := splitArgs(args)

	// setup usage metadata for deffered usage printing
	usage := &usage{
		deployerName:   deployerName,
		kubetest2Flags: kubetest2Flags,
	}

	// parse the kubetest2 common flags flags
	// NOTE: parseError should contain the first error from parsing.
	// We will later show this + usage if there is one
	parseError := kubetest2Flags.Parse(deployerArgs)

	// now that we've parsed flags we can look up the tester
	var newTester types.NewTester
	if opts.test != "" {
		n, testerUsage, ok := testers.Get(opts.test)
		newTester = n

		// if the tester exists, record usage info
		if ok {
			usage.testerUsage = testerUsage
			usage.testerName = opts.test
		} else if parseError == nil {
			// otherwise fail if the named tester does not exist
			// NOTE: we only retain the first parse error currently, and handle below
			// TODO(bentheelder): inform the user which testers exist
			parseError = errors.Errorf("no such tester: %#v", opts.test)
		}
	}

	// instantiate the deployer
	deployer, deployerFlags := newDeployer(opts)

	// capture deployer flags for usage
	usage.deployerFlags = deployerFlags

	// sanity check that the deployer did not register any identical flags
	deployerFlags.VisitAll(func(f *pflag.Flag) {
		if kubetest2Flags.Lookup(f.Name) != nil {
			panic(errors.Errorf("kubetest2 common flag %#v re-registered by deployer", f.Name))
		}
		if f.Shorthand != "" && kubetest2Flags.ShorthandLookup(f.Shorthand) != nil {
			panic(errors.Errorf("kubetest2 common shorthand flag %#v re-registered by deployer", f.Shorthand))
		}
	})

	// parse the combined deployer flags and kubetest2 flags
	allFlags := pflag.NewFlagSet(deployerName, pflag.ContinueOnError)
	allFlags.AddFlagSet(kubetest2Flags)
	allFlags.AddFlagSet(deployerFlags)
	if err := allFlags.Parse(deployerArgs); err != nil {
		// NOTE: we only retain the first parse error currently, and handle below
		if err != nil && parseError == nil {
			parseError = err
		}
	}

	// instantiate the tester if testing was specified
	// NOTE: we need to do this before building etc, so we can fail fast on
	// invalid options to the tester
	var tester types.Tester
	if newTester != nil {
		t, err := newTester(opts, testerArgs, deployer)
		tester = t

		// NOTE: we only retain the first parse error currently, and handle below
		if err != nil && parseError == nil {
			parseError = err
		}
	}

	// print usage and return if explicitly requested
	if opts.HelpRequested() {
		cmd.Print(usage.String())
		return nil
	}

	// otherwise if we encountered any errors with the user input
	// show the error / help, usage and then return
	if parseError != nil {
		// ensure this is an incorrect usage error so the top level
		// app logic will not print the error again, see Main()
		//
		// also make sure we print it here before the app usage either way
		if v, ok := parseError.(types.IncorrectUsage); ok {
			cmd.Print(v.HelpText())
		} else {
			incorrectUsageString := fmt.Sprintf("Error: %s", parseError)
			parseError = types.NewIncorrectUsage(incorrectUsageString)
			cmd.Print(incorrectUsageString)
		}
		// then print the actual usage
		cmd.Print("\n\n")
		cmd.Print(usage.String())
		return parseError
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

// splitArgs splits args into deployerArgs and testerArgs at the first bare `--`
func splitArgs(args []string) ([]string, []string) {
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
	return args, testArgs
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

// metadata used for CLI usage string
type usage struct {
	kubetest2Flags *pflag.FlagSet
	deployerFlags  *pflag.FlagSet
	deployerName   string
	testerName     string
	testerUsage    string
	// purely computed fields, see Default()
	deployerUsage string
}

func (u *usage) setDefaults() {
	u.deployerUsage = fmt.Sprintf("  NONE - %s has no flags", u.deployerName)
	if u.deployerFlags != nil {
		u.deployerUsage = u.deployerFlags.FlagUsages()
	}
	if u.testerUsage == "" {
		u.testerUsage = fmt.Sprintf("  NONE - %s has no usage", u.testerName)
	}
}

// helper to compute usage text
func (u *usage) String() string {
	// fixup any default values
	u.setDefaults()

	// build the usage string
	s := fmt.Sprintf(
		strings.TrimPrefix(`
Usage:
  kubetest2 %s [Flags] [DeployerFlags] -- [TesterArgs]

Flags:
%s
DeployerFlags(%s):
%s
`, "\n"),
		u.deployerName,
		u.kubetest2Flags.FlagUsages(),
		u.deployerName,
		u.deployerUsage,
	)

	// add tester info if we selected a tester and have it
	if u.testerName != "" {
		s += fmt.Sprintf(
			strings.TrimPrefix(`
TesterArgs(%s):
%s
`, "\n"),
			u.testerName,
			u.testerUsage,
		)
	}

	return s
}
