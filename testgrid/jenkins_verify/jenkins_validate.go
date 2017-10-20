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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	prow_config "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/testgrid/config/yaml2proto"
)

func main() {
	args := os.Args[1:]

	if len(args) != 3 {
		fmt.Println("Missing args - usage: go run jenkins_validate.go <path/to/job_collection> <path/to/prow> <path/to/testgrid_config>")
		os.Exit(1)
	}

	jobPath := args[0]
	prowPath := args[1]
	configPath := args[2]

	jobs := make(map[string]bool)
	files, err := filepath.Glob(jobPath + "/*")
	if err != nil {
		fmt.Println("Failed to collect outputs.")
		os.Exit(1)
	}

	for _, file := range files {
		file = strings.TrimPrefix(file, jobPath+"/")
		jobs[file] = false
	}

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Failed reading %v\n", configPath)
		os.Exit(1)
	}

	c := yaml2proto.Config{}
	if err := c.Update(data); err != nil {
		fmt.Printf("Failed to convert yaml to protobuf: %v\n", err)
		os.Exit(1)
	}

	config, err := c.Raw()
	if err != nil {
		fmt.Printf("Error validating config: %v\n", err)
		os.Exit(1)
	}

	prowConfig, err := prow_config.Load(prowPath + "/config.yaml")
	if err != nil {
		fmt.Printf("Could not load prow configs: %v\n", err)
		os.Exit(1)
	}

	// Also check k/k presubmit, prow postsubmit and periodic jobs
	for _, job := range prowConfig.AllPresubmits([]string{"jlewi/mlkube.io", "kubernetes/kubernetes", "kubernetes/test-infra", "kubernetes/cluster-registry"}) {
		jobs[job.Name] = false
	}

	for _, job := range prowConfig.AllPostsubmits([]string{}) {
		if job.Agent != "jenkins" {
			jobs[job.Name] = false
		}
	}

	for _, job := range prowConfig.AllPeriodics() {
		if job.Agent != "jenkins" {
			jobs[job.Name] = false
		}
	}

	// For now anything outsite k8s-jenkins/(pr-)logs are considered to be fine
	testgroups := make(map[string]bool)
	for _, testgroup := range config.TestGroups {
		if strings.Contains(testgroup.GcsPrefix, "kubernetes-jenkins/logs/") {
			job := strings.TrimPrefix(testgroup.GcsPrefix, "kubernetes-jenkins/logs/")
			testgroups[job] = false
		}

		if strings.Contains(testgroup.GcsPrefix, "kubernetes-jenkins/pr-logs/directory/") {
			job := strings.TrimPrefix(testgroup.GcsPrefix, "kubernetes-jenkins/pr-logs/directory/")
			testgroups[job] = false
		}
	}

	// Cross check
	// -- Each job need to have a match testgrid group
	for job := range jobs {
		if _, ok := testgroups[job]; ok {
			testgroups[job] = true
			jobs[job] = true
		}
	}

	// Conclusion
	badjobs := []string{}
	for job, valid := range jobs {
		if !valid {
			badjobs = append(badjobs, job)
			fmt.Printf("Job %v does not have a matching testgrid testgroup\n", job)
		}
	}

	badconfigs := []string{}
	for testgroup, valid := range testgroups {
		if !valid {
			badconfigs = append(badconfigs, testgroup)
			fmt.Printf("Testgrid group %v does not have a matching jenkins or prow job\n", testgroup)
		}
	}

	if len(badconfigs) > 0 {
		fmt.Printf("Total bad config(s) - %v\n", len(badconfigs))
	}

	if len(badjobs) > 0 {
		fmt.Printf("Total bad job(s) - %v\n", len(badjobs))
	}

	if len(badconfigs) > 0 || len(badjobs) > 0 {
		os.Exit(1)
	}
}
