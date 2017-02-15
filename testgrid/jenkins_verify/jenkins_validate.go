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

	"github.com/golang/protobuf/proto"
	prow_config "k8s.io/test-infra/prow/config"
	config "k8s.io/test-infra/testgrid/config/pb"
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

	jenkinsjobs := make(map[string]bool)
	files, err := filepath.Glob(jobPath + "/*")
	if err != nil {
		fmt.Println("Failed to collect outputs.")
		os.Exit(1)
	}

	for _, file := range files {
		file = strings.TrimPrefix(file, jobPath+"/")
		jenkinsjobs[file] = false
	}

	data, err := ioutil.ReadFile(configPath)
	protobufData, err := yaml2proto.Yaml2Proto(data)
	if err != nil {
		fmt.Printf("Failed to convert yaml to protobuf: %v\n", err)
		os.Exit(1)
	}

	config := &config.Configuration{}
	if err := proto.Unmarshal(protobufData, config); err != nil {
		fmt.Printf("Failed to parse config: %v\n", err)
		os.Exit(1)
	}

	prowConfig, err := prow_config.Load(prowPath + "/config.yaml")
	if err != nil {
		fmt.Printf("Could not load prow configs: %v\n", err)
		os.Exit(1)
	}

	// For now anything outsite k8s-jenkins/logs are considered to be fine
	testgroups := make(map[string]bool)
	for _, testgroup := range config.TestGroups {
		if strings.Contains(testgroup.GcsPrefix, "kubernetes-jenkins/logs/") {
			job := strings.TrimPrefix(testgroup.GcsPrefix, "kubernetes-jenkins/logs/")
			testgroups[job] = false
		}
	}

	// Cross check
	// -- Each jenkins job need to have a match testgrid group
	for jenkinsjob, _ := range jenkinsjobs {
		if _, ok := testgroups[jenkinsjob]; ok {
			testgroups[jenkinsjob] = true
			jenkinsjobs[jenkinsjob] = true
		}
	}

	// Also check prow jobs
	for _, job := range prowConfig.AllJobNames() {
		if _, ok := testgroups[job]; ok {
			testgroups[job] = true
			jenkinsjobs[job] = true
		}
	}

	// Conclusion
	badjobs := []string{}
	for jenkinsjob, valid := range jenkinsjobs {
		if !valid {
			badjobs = append(badjobs, jenkinsjob)
			fmt.Printf("Job %v does not have a matching testgrid testgroup\n", jenkinsjob)
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
		fmt.Printf("Total bad config - %v\n", len(badconfigs))
	}

	if len(badjobs) > 0 {
		fmt.Printf("Total bad jenkins job - %v\n", len(badjobs))
	}

	if len(badconfigs) > 0 || len(badjobs) > 0 {
		os.Exit(1)
	}
}
