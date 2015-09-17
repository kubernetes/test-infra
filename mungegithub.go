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

	"k8s.io/contrib/mungegithub/config"
	"k8s.io/contrib/mungegithub/issues"
	"k8s.io/contrib/mungegithub/pulls"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

var (
	_ = fmt.Print
)

func addMungeFlags(config *config.MungeConfig, cmd *cobra.Command) {
	cmd.Flags().BoolVar(&config.Once, "once", false, "If true, run one loop and exit")
	cmd.Flags().StringSliceVar(&config.IssueMungersList, "issue-mungers", []string{}, "A list of issue mungers to run")
	cmd.Flags().StringSliceVar(&config.PRMungersList, "pr-mungers", []string{}, "A list of pull request mungers to run")
	cmd.Flags().DurationVar(&config.Period, "period", 30*time.Minute, "The period for running mungers")
}

func doMungers(config *config.MungeConfig) error {
	if len(config.IssueMungers) == 0 && len(config.PRMungersList) == 0 {
		glog.Fatalf("must include at least one --issue-mungers or --pr-mungers")
	}
	for {
		nextRunStartTime := time.Now().Add(config.Period)
		if len(config.IssueMungers) > 0 {
			glog.Infof("Running issue mungers")
			if err := issues.MungeIssues(config); err != nil {
				glog.Errorf("Error munging issues: %v", err)
			}
		}
		if len(config.PRMungersList) > 0 {
			glog.Infof("Running PR mungers")
			if err := pulls.MungePullRequests(config); err != nil {
				glog.Errorf("Error munging PRs: %v", err)
			}
		}
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
	config := &config.MungeConfig{}
	root := &cobra.Command{
		Use:   filepath.Base(os.Args[0]),
		Short: "A program to add labels, check tests, and generally mess with outstanding PRs",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := config.PreExecute(); err != nil {
				return err
			}
			return doMungers(config)
		},
	}
	config.AddRootFlags(root)
	addMungeFlags(config, root)

	prMungers := pulls.GetAllMungers()
	for _, m := range prMungers {
		m.AddFlags(root)
	}

	issueMungers := issues.GetAllMungers()
	for _, m := range issueMungers {
		m.AddFlags(root)
	}

	if err := root.Execute(); err != nil {
		glog.Fatalf("%v\n", err)
	}
}
