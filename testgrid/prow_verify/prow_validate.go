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
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	prow_config "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/testgrid/config/yaml2proto"
)

var jenkinsPath = flag.String("jenkins-config", "jenkins/job-configs", "path to jenkins config dir")
var prowPath = flag.String("prow-config", "prow/config.yaml", "path to prow config file")
var configPath = flag.String("testgrid-config", "testgrid/config/config.yaml", "path to testgrid config file")

func main() {
	flag.Parse()
	jobs := make(map[string]bool)
	// TODO(krzyzacy): delete all the Jenkins stuff here after kill Jenkins
	// workaround for special jenkins jobs does not follow job-name: pattern:
	jobs["maintenance-all-hourly"] = false
	jobs["maintenance-all-daily"] = false
	jobs["kubernetes-update-jenkins-jobs"] = false

	if err := filepath.Walk(*jenkinsPath, func(path string, file os.FileInfo, err error) error {
		if !file.IsDir() {
			file, err := os.Open(path)
			defer file.Close()

			if err != nil {
				return err
			}
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				if strings.Contains(scanner.Text(), "job-name:") {
					job := strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "job-name: ")
					jobs[job] = false
				}
			}
		}
		return nil
	}); err != nil {
		fmt.Printf("Failed parsing Jenkins config %v\n", err)
		os.Exit(1)
	}

	data, err := ioutil.ReadFile(*configPath)
	if err != nil {
		fmt.Printf("Failed reading %v : %v\n", *configPath, err)
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

	prowConfig, err := prow_config.Load(*prowPath)
	if err != nil {
		fmt.Printf("Could not load prow configs: %v\n", err)
		os.Exit(1)
	}

	// Also check k/k presubmit, prow postsubmit and periodic jobs
	for _, job := range prowConfig.AllPresubmits([]string{
		"google/kubeflow",
		"kubernetes/kubernetes",
		"kubernetes/test-infra",
		"kubernetes/cluster-registry",
		"kubernetes/federation",
		"tensorflow/k8s",
	}) {
		jobs[job.Name] = false
	}

	for _, job := range prowConfig.AllPostsubmits([]string{}) {
		jobs[job.Name] = false
	}

	for _, job := range prowConfig.AllPeriodics() {
		jobs[job.Name] = false
	}

	// For now anything outsite k8s-jenkins/(pr-)logs are considered to be fine
	testgroups := make(map[string]bool)
	for _, testgroup := range config.TestGroups {
		if strings.Contains(testgroup.GcsPrefix, "kubernetes-jenkins/logs/") {
			// The convention is that the job name is the final part of the GcsPrefix
			job := filepath.Base(testgroup.GcsPrefix)
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
