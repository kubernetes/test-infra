/*
Copyright 2016 The Kubernetes Authors.

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
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/spf13/cobra"

	utilflag "k8s.io/apiserver/pkg/util/flag"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/options"
)

// MungeIssue is the real worker. It is called for every open github Issue
// But note that all PRs are Issues. (Not all Issues are PRs.) This particular
// function ignores all issues that are not PRs and prints the number for the
// PRs.
func MungeIssue(obj *github.MungeObject) error {
	if !obj.IsPR() {
		return nil
	}
	glog.Infof("PR: %d", *obj.Issue.Number)
	return nil
}

func main() {
	config := &github.Config{}
	root := &cobra.Command{
		Use:   filepath.Base(os.Args[0]),
		Short: "A program to convert blunderbuss.yaml",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := config.PreExecute(); err != nil {
				return err
			}
			if err := config.ForEachIssueDo(MungeIssue); err != nil {
				glog.Errorf("Error munging PRs: %v", err)
			}
			return nil
		},
	}
	root.SetGlobalNormalizationFunc(utilflag.WordSepNormalizeFunc)
	config.RegisterOptions(options.New()) // Always uses defaults since Load is never called.
	root.Execute()
}
