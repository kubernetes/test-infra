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
	"strings"
	"testing"

	"path/filepath"

	"k8s.io/apimachinery/pkg/util/sets"

	prow_config "k8s.io/test-infra/prow/config"
	config_pb "k8s.io/test-infra/testgrid/config"
)

type SQConfig struct {
	Data map[string]string `yaml:"data,omitempty"`
}

var (
	companies = []string{
		"canonical",
		"cos",
		"cri-o",
		"istio",
		"google",
		"kopeio",
		"redhat",
		"vmware",
	}
	orgs = []string{
		"conformance",
		"presubmits",
		"sig",
		"wg",
	}
	prefixes = [][]string{orgs, companies}
)

// Shared testgrid config, loaded at TestMain.
var cfg *config_pb.Configuration

func TestMain(m *testing.M) {
	//make sure we can parse config.yaml
	yamlData, err := ioutil.ReadFile("../../config.yaml")
	if err != nil {
		fmt.Printf("IO Error : Cannot Open File config.yaml")
		os.Exit(1)
	}

	c := Config{}
	if err := c.Update(yamlData); err != nil {
		fmt.Printf("Yaml2Proto - Conversion Error %v", err)
		os.Exit(1)
	}

	cfg, err = c.Raw()
	if err != nil {
		fmt.Printf("Error validating config: %v", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestConfig(t *testing.T) {
	// testgroup - occurrence map, validate testgroups
	testgroupMap := make(map[string]int32)

	for testgroupidx, testgroup := range cfg.TestGroups {
		// All testgroup must have a name and a query
		if testgroup.Name == "" || testgroup.GcsPrefix == "" {
			t.Errorf("Testgroup #%v (Name: '%v', Query: '%v'): - Must have a name and query",
				testgroupidx, testgroup.Name, testgroup.GcsPrefix)
		}

		// All testgroup must not have duplicated names
		if testgroupMap[testgroup.Name] > 0 {
			t.Errorf("Duplicated Testgroup: %v", testgroup.Name)
		} else {
			testgroupMap[testgroup.Name] = 1
		}

		if !testgroup.IsExternal {
			t.Errorf("Testgroup %v: IsExternal should always be true!", testgroup.Name)
		}
		if !testgroup.UseKubernetesClient {
			t.Errorf("Testgroup %v: UseKubernetesClient should always be true!", testgroup.Name)
		}

		if strings.HasPrefix(testgroup.GcsPrefix, "kubernetes-jenkins/logs/") {
			// The expectation is that testgroup.Name is the name of a Prow job and the GCSPrefix
			// follows the convention kubernetes-jenkins/logs/.../jobName
			// The final part of the prefix should be the job name.
			expected := filepath.Join(filepath.Dir(testgroup.GcsPrefix), testgroup.Name)
			if expected != testgroup.GcsPrefix {
				t.Errorf("Kubernetes Testgroup %v GcsPrefix; Got %v; Want %v", testgroup.Name, testgroup.GcsPrefix, expected)
			}
		}

		if testgroup.TestNameConfig != nil {
			if testgroup.TestNameConfig.NameFormat == "" {
				t.Errorf("Testgroup %v: NameFormat must not be empty!", testgroup.Name)
			}

			if len(testgroup.TestNameConfig.NameElements) != strings.Count(testgroup.TestNameConfig.NameFormat, "%") {
				t.Errorf("Testgroup %v: TestNameConfig must have number NameElement equal to format count in NameFormat!", testgroup.Name)
			}
		}

		// All PR testgroup has num_columns_recent equals 20
		if strings.HasPrefix(testgroup.GcsPrefix, "kubernetes-jenkins/pr-logs/directory/") {
			if testgroup.NumColumnsRecent < 20 {
				t.Errorf("Testgroup %v: num_columns_recent: must be greater than 20 for presubmit jobs!", testgroup.Name)
			}
		}
	}

	// dashboard name set
	dashboardmap := make(map[string]bool)

	for dashboardidx, dashboard := range cfg.Dashboards {
		// All dashboard must have a name
		if dashboard.Name == "" {
			t.Errorf("Dashboard %v: - Must have a name", dashboardidx)
		}

		found := false
		for _, kind := range prefixes {
			for _, prefix := range kind {
				if strings.HasPrefix(dashboard.Name, prefix+"-") || dashboard.Name == prefix {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("Dashboard %v: must prefix with one of: %v", dashboard.Name, prefixes)
		}

		// All dashboard must not have duplicated names
		if dashboardmap[dashboard.Name] {
			t.Errorf("Duplicated dashboard: %v", dashboard.Name)
		} else {
			dashboardmap[dashboard.Name] = true
		}

		// All dashboard must have at least one tab
		if len(dashboard.DashboardTab) == 0 {
			t.Errorf("Dashboard %v: - Must have more than one dashboardtab", dashboard.Name)
		}

		// dashboardtabSet is a set that checks duplicate tab name within each dashboard
		dashboardtabSet := sets.NewString()

		// dashboardtestgroupSet is a set that checks duplicate testgroups within each dashboard
		dashboardtestgroupSet := sets.NewString()

		// All notifications in dashboard must have a summary
		if len(dashboard.Notifications) != 0 {
			for notificationindex, notification := range dashboard.Notifications {
				if notification.Summary == "" {
					t.Errorf("Notification %v in dashboard %v: - Must have a summary", notificationindex, dashboard.Name)
				}
			}
		}

		for tabindex, dashboardtab := range dashboard.DashboardTab {

			// All dashboardtab must have a name and a testgroup
			if dashboardtab.Name == "" || dashboardtab.TestGroupName == "" {
				t.Errorf("Dashboard %v, tab %v: - Must have a name and a testgroup name", dashboard.Name, tabindex)
			}

			// All dashboardtab within a dashboard must not have duplicated names
			if dashboardtabSet.Has(dashboardtab.Name) {
				t.Errorf("Duplicated name in dashboard %s: %v", dashboard.Name, dashboardtab.Name)
			} else {
				dashboardtabSet.Insert(dashboardtab.Name)
			}

			// All dashboardtab within a dashboard must not have duplicated testgroupnames
			if dashboardtestgroupSet.Has(dashboardtab.TestGroupName) {
				t.Errorf("Duplicated testgroupnames in dashboard %s: %v", dashboard.Name, dashboardtab.TestGroupName)
			} else {
				dashboardtestgroupSet.Insert(dashboardtab.TestGroupName)
			}

			// All testgroup in dashboard must be defined in testgroups
			if testgroupMap[dashboardtab.TestGroupName] == 0 {
				t.Errorf("Dashboard %v, tab %v: - Testgroup %v must be defined first",
					dashboard.Name, dashboardtab.Name, dashboardtab.TestGroupName)
			} else {
				testgroupMap[dashboardtab.TestGroupName]++
			}

			if dashboardtab.AlertOptions != nil && (dashboardtab.AlertOptions.AlertStaleResultsHours != 0 || dashboardtab.AlertOptions.NumFailuresToAlert != 0) {
				for _, testgroup := range cfg.TestGroups {
					// Disallow alert options in tab but not group.
					// Disallow different alert options in tab vs. group.
					if testgroup.Name == dashboardtab.TestGroupName {
						if testgroup.AlertStaleResultsHours == 0 {
							t.Errorf("Cannot define alert_stale_results_hours in DashboardTab %v and not TestGroup %v.", dashboardtab.Name, dashboardtab.TestGroupName)
						}
						if testgroup.NumFailuresToAlert == 0 {
							t.Errorf("Cannot define num_failures_to_alert in DashboardTab %v and not TestGroup %v.", dashboardtab.Name, dashboardtab.TestGroupName)
						}
						if testgroup.AlertStaleResultsHours != dashboardtab.AlertOptions.AlertStaleResultsHours {
							t.Errorf("alert_stale_results_hours for DashboardTab %v must match TestGroup %v.", dashboardtab.Name, dashboardtab.TestGroupName)
						}
						if testgroup.NumFailuresToAlert != dashboardtab.AlertOptions.NumFailuresToAlert {
							t.Errorf("num_failures_to_alert for DashboardTab %v must match TestGroup %v.", dashboardtab.Name, dashboardtab.TestGroupName)
						}
					}
				}
			}
		}
	}

	// No dup of dashboard groups, and no dup dashboard in a dashboard group
	groups := make(map[string]bool)
	tabs := make(map[string]string)

	for idx, dashboardGroup := range cfg.DashboardGroups {
		// All dashboard must have a name
		if dashboardGroup.Name == "" {
			t.Errorf("DashboardGroup %v: - DashboardGroup must have a name", idx)
		}

		found := false
		for _, kind := range prefixes {
			for _, prefix := range kind {
				if strings.HasPrefix(dashboardGroup.Name, prefix+"-") || prefix == dashboardGroup.Name {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("Dashboard group %v: must prefix with one of: %v", dashboardGroup.Name, prefixes)
		}

		// All dashboardgroup must not have duplicated names
		if _, ok := groups[dashboardGroup.Name]; ok {
			t.Errorf("Duplicated dashboard: %v", dashboardGroup.Name)
		} else {
			groups[dashboardGroup.Name] = true
		}

		if _, ok := dashboardmap[dashboardGroup.Name]; ok {
			t.Errorf("%v is both a dashboard and dashboard group name.", dashboardGroup.Name)
		}

		for _, dashboard := range dashboardGroup.DashboardNames {
			// All dashboard must not have duplicated names
			if exist, ok := tabs[dashboard]; ok {
				t.Errorf("Duplicated dashboard %v in dashboard group %v and %v", dashboard, exist, dashboardGroup.Name)
			} else {
				tabs[dashboard] = dashboardGroup.Name
			}

			if _, ok := dashboardmap[dashboard]; !ok {
				t.Errorf("Dashboard %v needs to be defined before adding to a dashboard group!", dashboard)
			}

			if !strings.HasPrefix(dashboard, dashboardGroup.Name+"-") {
				t.Errorf("Dashboard %v in group %v must have the group name as a prefix", dashboard, dashboardGroup.Name)
			}
		}
	}

	// All Testgroup should be mapped to one or more tabs
	for testgroupname, occurrence := range testgroupMap {
		if occurrence == 1 {
			t.Errorf("Testgroup %v - defined but not used in any dashboards", testgroupname)
		}
	}
}

func TestJobsTestgridEntryMatch(t *testing.T) {
	prowPath := "../../../prow/config.yaml"
	jobPath := "../../../config/jobs"

	jobs := make(map[string]bool)

	prowConfig, err := prow_config.Load(prowPath, jobPath)
	if err != nil {
		t.Fatalf("Could not load prow configs: %v\n", err)
	}

	// Also check k/k presubmit, prow postsubmit and periodic jobs
	for _, job := range prowConfig.AllPresubmits([]string{
		"bazelbuild/rules_k8s",
		"google/cadvisor",
		"helm/charts",
		"GoogleCloudPlatform/k8s-cluster-bundle",
		"kubeflow/arena",
		"kubeflow/caffe2-operator",
		"kubeflow/chainer-operator",
		"kubeflow/examples",
		"kubeflow/experimental-beagle",
		"kubeflow/experimental-kvc",
		"kubeflow/experimental-seldon",
		"kubeflow/katib",
		"kubeflow/kubebench",
		"kubeflow/kubeflow",
		"kubeflow/mpi-operator",
		"kubeflow/mxnet-operator",
		"kubeflow/pytorch-operator",
		"kubeflow/reporting",
		"kubeflow/testing",
		"kubeflow/tf-operator",
		"kubeflow/website",
		"kubernetes/cloud-provider-aws",
		"kubernetes/cloud-provider-vsphere",
		"kubernetes/cluster-registry",
		"kubernetes/federation",
		"kubernetes/kops",
		"kubernetes/kubernetes",
		"kubernetes/org",
		"kubernetes/publishing-bot",
		"kubernetes/test-infra",
		"kubernetes-sigs/aws-alb-ingress-controller",
		"kubernetes-sigs/aws-ebs-csi-driver",
		"kubernetes-sigs/cluster-api",
		"kubernetes-sigs/cluster-api-provider-aws",
		"kubernetes-sigs/cluster-api-provider-digitalocean",
		"kubernetes-sigs/cluster-api-provider-gcp",
		"kubernetes-sigs/cluster-api-provider-vsphere",
		"kubernetes-sigs/cluster-api-provider-openstack",
		"kubernetes-sigs/poseidon",
		"kubernetes-sigs/structured-merge-diff",
		"tensorflow/minigo",
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
	for _, testgroup := range cfg.TestGroups {
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
		t.Fatal("Failed with invalid config or job entries")
	}
}
