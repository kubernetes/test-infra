/*
Copyright 2015 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	utilflag "k8s.io/apiserver/pkg/util/flag"
	"k8s.io/test-infra/mungegithub/features"
	github_util "k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungeopts"
	"k8s.io/test-infra/mungegithub/mungers"
	"k8s.io/test-infra/mungegithub/options"
	"k8s.io/test-infra/mungegithub/reports"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

type mungeConfig struct {
	github_util.Config
	*options.Options
	features.Features

	path string

	once           bool
	prMungers      []string
	issueReports   []string
	period         time.Duration
	webhookKeyFile string
}

func registerOptions(config *mungeConfig) {
	config.RegisterBool(&config.once, "once", false, "If true, run one loop and exit")
	config.RegisterStringSlice(&config.prMungers, "pr-mungers", []string{}, "A list of pull request mungers to run")
	config.RegisterStringSlice(&config.issueReports, "issue-reports", []string{}, "A list of issue reports to run. If set, will run the reports and exit.")
	config.RegisterDuration(&config.period, "period", 10*time.Minute, "The period for running mungers")
	config.RegisterString(&config.webhookKeyFile, "github-key-file", "", "Github secret key for webhooks")

	// options that necessitate a full restart when they change.
	immutables := sets.NewString("once", "pr-mungers", "issue-reports", "github-key-file")
	// Register config update callback. This goes before registration of other sources to ensure:
	// 1) If an immutable option is changed, we fail before calling other update callbacks.
	// 2) The updated config is printed before anything else when a new config is found.
	config.RegisterUpdateCallback(func(changed sets.String) error {
		if common := immutables.Intersection(changed); len(common) > 0 {
			return fmt.Errorf("option key(s) %q was updated necessitating a restart", common.List())
		}

		glog.Infof(
			"ConfigMap '%s' was updated.\nOptions changed: %q\n%s",
			config.path,
			changed.List(),
			config.CurrentValues(),
		)
		return nil
	})

	// Register options from other sources.
	// github.Config
	immutables = immutables.Union(config.Config.RegisterOptions(config.Options))
	// Features (per feature opts)
	immutables = immutables.Union(config.Features.RegisterOptions(config.Options))
	// Reports (per report opts)
	immutables = immutables.Union(reports.RegisterOptions(config.Options))
	// MungeOpts (opts shared by mungers or features)
	immutables = immutables.Union(mungeopts.RegisterOptions(config.Options))
	// Mungers (per munger opts)
	immutables = immutables.Union(mungers.RegisterOptions(config.Options))
}

func doMungers(config *mungeConfig) error {
	for {
		nextRunStartTime := time.Now().Add(config.period)
		glog.Infof("Running mungers")
		config.NextExpectedUpdate(nextRunStartTime)

		config.Features.EachLoop()
		mungers.EachLoop()

		if err := config.ForEachIssueDo(mungers.MungeIssue); err != nil {
			glog.Errorf("Error munging PRs: %v", err)
		}

		config.ResetAPICount()
		if config.once {
			break
		}
		if nextRunStartTime.After(time.Now()) {
			sleepDuration := nextRunStartTime.Sub(time.Now())
			glog.Infof("Sleeping for %v\n", sleepDuration)
			time.Sleep(sleepDuration)
		} else {
			glog.Infof("Not sleeping as we took more than %v to complete one loop\n", config.period)
		}

		if config.path == "" {
			continue
		}
		if _, err := config.Load(config.path); err != nil {
			if _, ok := err.(*options.UpdateCallbackError); ok {
				return err // Fatal
			}
			// Non-fatal since the config has previously been loaded successfully.
			glog.Errorf("Error reloading config (ignored): %v", err)
		}
	}
	return nil
}

func main() {
	config := &mungeConfig{}
	root := &cobra.Command{
		Use:   filepath.Base(os.Args[0]),
		Short: "A program to add labels, check tests, and generally mess with outstanding PRs",
		RunE: func(_ *cobra.Command, _ []string) error {
			optFlagsSpecified := config.Options.FlagsSpecified()
			if config.path != "" && len(optFlagsSpecified) > 0 {
				glog.Fatalf("Error: --config-path flag cannot be used with option flags. Option flag(s) %v were specified.", optFlagsSpecified.List())
			}
			if config.path != "" {
				glog.Infof("Loading config from file '%s'.\n", config.path)
				if _, err := config.Load(config.path); err != nil {
					glog.Fatalf("Error loading options: %v", err)
				}
			} else {
				glog.Info("Loading config from flags.\n")
				config.PopulateFromFlags()
			}

			glog.Info(config.CurrentValues())
			if err := config.PreExecute(); err != nil {
				return err
			}
			if len(config.issueReports) > 0 {
				return reports.RunReports(&config.Config, config.issueReports...)
			}
			if len(config.prMungers) == 0 {
				glog.Fatalf("must include at least one --pr-mungers")
			}
			if err := mungers.RegisterMungers(config.prMungers); err != nil {
				glog.Fatalf("unable to find requested mungers: %v", err)
			}
			// Include the server feature if webhooks are enabled.
			requestedFeatures := sets.NewString(mungers.RequestedFeatures()...)
			if config.webhookKeyFile != "" {
				requestedFeatures = requestedFeatures.Union(sets.NewString(features.ServerFeatureName))
			}
			if err := config.Features.Initialize(&config.Config, requestedFeatures.List()); err != nil {
				return err
			}
			if err := mungers.InitializeMungers(&config.Config, &config.Features); err != nil {
				glog.Fatalf("unable to initialize mungers: %v", err)
			}
			if config.webhookKeyFile != "" {
				config.HookHandler = github_util.NewWebHookAndListen(config.webhookKeyFile, config.Features.Server)
			}
			return doMungers(config)
		},
	}

	config.Options = options.New()
	registerOptions(config)
	config.Options.ToFlags()

	root.SetGlobalNormalizationFunc(utilflag.WordSepNormalizeFunc)

	// Command line flags.
	flag.BoolVar(&config.DryRun, "dry-run", true, "If true, don't actually merge anything")
	flag.StringVar(&config.path, "config-path", "", "File path to yaml config map containing the values to use for options.")
	root.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	if err := root.Execute(); err != nil {
		glog.Fatalf("%v\n", err)
	}
}
