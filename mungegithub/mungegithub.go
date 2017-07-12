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
	"os"
	"path/filepath"
	"time"

	utilflag "k8s.io/kubernetes/pkg/util/flag"
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

	// Print the options whenever they are updated.
	// This goes before registration of other sources to ensure the updated config is printed first
	// when a new config is found.
	config.RegisterUpdateCallback(func() {
		glog.Infof("ConfigMap '%s' was updated.\n%s", config.path, config.CurrentValues())
	})

	// Register options from other sources.
	config.Config.RegisterOptions(config.Options)   // github.Config
	config.Features.RegisterOptions(config.Options) // Features (per feature opts)
	reports.RegisterOptions(config.Options)         // Reports (per report opts)
	mungers.RegisterOptions(config.Options)         // Mungers (per munger opts)
	mungeopts.RegisterOptions(config.Options)       // MungeOpts (opts shared by mungers or features)
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
		// Uncommenting will make configmap reload if changed.
		// config.Load()
	}
	return nil
}

func main() {
	config := &mungeConfig{}
	root := &cobra.Command{
		Use:   filepath.Base(os.Args[0]),
		Short: "A program to add labels, check tests, and generally mess with outstanding PRs",
		RunE: func(_ *cobra.Command, _ []string) error {
			config.Options = options.New(config.path)
			registerOptions(config)
			config.Load()

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
			requestedFeatures := mungers.RequestedFeatures()
			if err := config.Features.Initialize(&config.Config, requestedFeatures); err != nil {
				return err
			}
			if err := mungers.InitializeMungers(&config.Config, &config.Features); err != nil {
				glog.Fatalf("unable to initialize mungers: %v", err)
			}
			if config.webhookKeyFile != "" {
				config.HookHandler = github_util.NewWebHookAndListen(config.webhookKeyFile, mungeopts.Server.Address)
			}
			return doMungers(config)
		},
	}
	root.SetGlobalNormalizationFunc(utilflag.WordSepNormalizeFunc)

	// Command line flags.
	root.Flags().BoolVar(&config.DryRun, "dry-run", true, "If true, don't actually merge anything")
	root.Flags().StringVar(&config.path, "config-path", "", "File path to yaml config map containing the values to use for options.")
	root.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	if err := root.Execute(); err != nil {
		glog.Fatalf("%v\n", err)
	}
}
