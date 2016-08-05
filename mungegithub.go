/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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
	"os"
	"path/filepath"
	"time"

	"k8s.io/contrib/mungegithub/features"
	github_util "k8s.io/contrib/mungegithub/github"
	"k8s.io/contrib/mungegithub/mungers"
	"k8s.io/contrib/mungegithub/reports"
	utilflag "k8s.io/kubernetes/pkg/util/flag"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/contrib/mungegithub/mungers/mungerutil"
)

var (
	_ = fmt.Print
)

type mungeConfig struct {
	github_util.Config
	MinIssueNumber   int
	PRMungersList    []string
	IssueReportsList []string
	Once             bool
	Period           time.Duration
	features.Features
}

func addMungeFlags(config *mungeConfig, cmd *cobra.Command) {
	cmd.Flags().BoolVar(&config.Once, "once", false, "If true, run one loop and exit")
	cmd.Flags().StringSliceVar(&config.PRMungersList, "pr-mungers", []string{}, "A list of pull request mungers to run")
	cmd.Flags().StringSliceVar(&config.IssueReportsList, "issue-reports", []string{}, "A list of issue reports to run. If set, will run the reports and exit.")
	cmd.Flags().DurationVar(&config.Period, "period", 10*time.Minute, "The period for running mungers")
}

func doMungers(config *mungeConfig) error {
	for {
		nextRunStartTime := time.Now().Add(config.Period)
		glog.Infof("Running mungers")
		config.NextExpectedUpdate(nextRunStartTime)

		config.Features.EachLoop()
		mungers.EachLoop()

		if err := config.ForEachIssueDo(mungers.MungeIssue); err != nil {
			glog.Errorf("Error munging PRs: %v", err)
		}
		config.ResetAPICount()
		if config.Once {
			break
		}
		if nextRunStartTime.After(time.Now()) {
			sleepDuration := nextRunStartTime.Sub(time.Now())
			glog.Infof("Sleeping for %v\n", sleepDuration)
			time.Sleep(sleepDuration)
		} else {
			glog.Infof("Not sleeping as we took more than %v to complete one loop\n", config.Period)
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
			glog.Info(mungerutil.PrettyString(config))
			if err := config.PreExecute(); err != nil {
				return err
			}
			if len(config.IssueReportsList) > 0 {
				return reports.RunReports(&config.Config, config.IssueReportsList...)
			}
			if len(config.PRMungersList) == 0 {
				glog.Fatalf("must include at least one --pr-mungers")
			}
			if err := mungers.RegisterMungers(config.PRMungersList); err != nil {
				glog.Fatalf("unable to find requested mungers: %v", err)
			}
			requestedFeatures := mungers.RequestedFeatures()
			if err := config.Features.Initialize(&config.Config, requestedFeatures); err != nil {
				return err
			}
			if err := mungers.InitializeMungers(&config.Config, &config.Features); err != nil {
				glog.Fatalf("unable to initialize mungers: %v", err)
			}
			return doMungers(config)
		},
	}
	root.SetGlobalNormalizationFunc(utilflag.WordSepNormalizeFunc)
	config.AddRootFlags(root)
	addMungeFlags(config, root)
	config.Features.AddFlags(root)

	allMungers := mungers.GetAllMungers()
	for _, m := range allMungers {
		m.AddFlags(root, &config.Config)
	}

	allReports := reports.GetAllReports()
	for _, r := range allReports {
		r.AddFlags(root, &config.Config)
	}

	if err := root.Execute(); err != nil {
		glog.Fatalf("%v\n", err)
	}
}
