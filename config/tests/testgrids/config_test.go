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

package testgrids

import (
	"context"
	"flag"
	"fmt"
	"net/mail"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/GoogleCloudPlatform/testgrid/config"
	config_pb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"k8s.io/test-infra/testgrid/pkg/configurator/configurator"
	"k8s.io/test-infra/testgrid/pkg/configurator/options"
	prow_config "sigs.k8s.io/prow/pkg/config"
	"sigs.k8s.io/prow/pkg/flagutil"

	configflagutil "sigs.k8s.io/prow/pkg/flagutil/config"
)

type SQConfig struct {
	Data map[string]string `yaml:"data,omitempty"`
}

var (
	companies = []string{
		"amazon",
		"canonical",
		"cos",
		"containerd",
		"cri-o",
		"istio",
		"googleoss",
		"google",
		"kopeio",
		"redhat",
		"ibm",
		"vmware",
		"gardener",
		"jetstack",
		"kubevirt",
	}
	orgs = []string{
		"conformance",
		"kops",
		"presubmits",
		"sig",
		"wg",
		"provider",
		"kubernetes-clients",
		"kcp",
	}
	dashboardPrefixes = [][]string{orgs, companies}

	// gcs prefixes populated by the kubernetes prow instance
	prowGcsPrefixes = []string{
		"kubernetes-jenkins/logs/",
		"kubernetes-jenkins/pr-logs/directory/",
		"kubernetes-ci-logs/logs/",
		"kubernetes-ci-logs/pr-logs/directory/",
	}
)

var defaultInputs options.MultiString = []string{"../../testgrids"}
var prowPath = flag.String("prow-config", "../../../config/prow/config.yaml", "Path to prow config")
var jobPath = flag.String("job-config", "../../jobs", "Path to prow job config")
var defaultYAML = flag.String("default", "../../testgrids/default.yaml", "Default yaml for testgrid")
var inputs options.MultiString
var protoPath = flag.String("config", "", "Path to TestGrid config proto")

// Shared testgrid config, loaded at TestMain.
var cfg *config_pb.Configuration

// Shared prow config, loaded at Test Main
var prowConfig *prow_config.Config

func TestMain(m *testing.M) {
	flag.Var(&inputs, "yaml", "comma-separated list of input YAML files or directories")
	flag.Parse()
	if *protoPath == "" {
		if len(inputs) == 0 {
			inputs = defaultInputs
		}
		// Generate proto from testgrid config
		tmpDir, err := os.MkdirTemp("", "testgrid-config-test")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer os.RemoveAll(tmpDir)
		tmpFile := path.Join(tmpDir, "test-proto")

		opt := options.Options{
			Inputs: inputs,
			ProwConfig: configflagutil.ConfigOptions{
				ConfigPath:    *prowPath,
				JobConfigPath: *jobPath,
			},
			DefaultYAML:     *defaultYAML,
			Output:          flagutil.NewStringsBeenSet(tmpFile),
			Oneshot:         true,
			StrictUnmarshal: true,
		}

		if err := configurator.RealMain(&opt); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		protoPath = &tmpFile
	}

	var err error
	cfg, err = config.Read(context.Background(), *protoPath, nil)
	if err != nil {
		fmt.Printf("Could not load config: %v\n", err)
		os.Exit(1)
	}

	prowConfig, err = prow_config.Load(*prowPath, *jobPath, nil, "")
	if err != nil {
		fmt.Printf("Could not load prow configs: %v\n", err)
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
			t.Errorf("Testgroup #%v (Name: '%v', GcsPrefix: '%v'): - Must have a name and gcs_prefix",
				testgroupidx, testgroup.Name, testgroup.GcsPrefix)
		}

		// All testgroup must not have duplicated names
		if testgroupMap[testgroup.Name] > 0 {
			t.Errorf("Duplicated Testgroup: %v", testgroup.Name)
		} else {
			testgroupMap[testgroup.Name] = 1
		}

		t.Run("Testgroup "+testgroup.Name, func(t *testing.T) {
			if !testgroup.IsExternal {
				t.Error("IsExternal must be true")
			}

			for hIdx, header := range testgroup.ColumnHeader {
				if header.ConfigurationValue == "" {
					t.Errorf("Column Header %d is empty", hIdx)
				}
			}

			for _, prowGcsPrefix := range prowGcsPrefixes {
				if strings.Contains(testgroup.GcsPrefix, prowGcsPrefix) {
					// The expectation is that testgroup.Name is the name of a Prow job and the GCSPrefix
					// follows the convention kubernetes-ci-logs/logs/.../jobName
					// The final part of the prefix should be the job name.
					expected := filepath.Join(filepath.Dir(testgroup.GcsPrefix), testgroup.Name)
					if expected != testgroup.GcsPrefix {
						t.Errorf("GcsPrefix: Got %s; Want %s", testgroup.GcsPrefix, expected)
					}
					break // out of prowGcsPrefix for loop
				}
			}

			if testgroup.TestNameConfig != nil {
				if testgroup.TestNameConfig.NameFormat == "" {
					t.Error("Empty NameFormat")
				}

				if got, want := len(testgroup.TestNameConfig.NameElements), strings.Count(testgroup.TestNameConfig.NameFormat, "%"); got != want {
					t.Errorf("TestNameConfig has %d elements, format %s wants %d", got, testgroup.TestNameConfig.NameFormat, want)
				}
			}

			// All PR testgroup has num_columns_recent equals 20
			if strings.HasPrefix(testgroup.GcsPrefix, "kubernetes-jenkins/pr-logs/directory/") || strings.HasPrefix(testgroup.GcsPrefix, "kubernetes-ci-logs/pr-logs/directory/") {
				if testgroup.NumColumnsRecent < 20 {
					t.Errorf("presubmit num_columns_recent want >=20, got %d", testgroup.NumColumnsRecent)
				}
			}
		})
	}

	// dashboard name set
	dashboardSet := sets.NewString()

	for dashboardidx, dashboard := range cfg.Dashboards {
		// All dashboard must have a name
		if dashboard.Name == "" {
			t.Errorf("Dashboard %v: - Must have a name", dashboardidx)
		}

		found := false
		for _, kind := range dashboardPrefixes {
			for _, prefix := range kind {
				if strings.HasPrefix(dashboard.Name, prefix) || dashboard.Name == prefix {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("Dashboard %v: must prefix with one of: %v", dashboard.Name, dashboardPrefixes)
		}

		// All dashboard must not have duplicated names
		if dashboardSet.Has(dashboard.Name) {
			t.Errorf("Duplicated dashboard: %v", dashboard.Name)
		} else {
			dashboardSet.Insert(dashboard.Name)
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
		}
	}

	// No dup of dashboard groups, and no dup dashboard in a dashboard group
	groupSet := sets.NewString()
	dashboardToGroupMap := make(map[string]string)

	for idx, dashboardGroup := range cfg.DashboardGroups {
		// All dashboard must have a name
		if dashboardGroup.Name == "" {
			t.Errorf("DashboardGroup %v: - DashboardGroup must have a name", idx)
		}

		found := false
		for _, kind := range dashboardPrefixes {
			for _, prefix := range kind {
				if strings.HasPrefix(dashboardGroup.Name, prefix) || prefix == dashboardGroup.Name {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("Dashboard group %v: must prefix with one of: %v", dashboardGroup.Name, dashboardPrefixes)
		}

		// All dashboardgroup must not have duplicated names
		if groupSet.Has(dashboardGroup.Name) {
			t.Errorf("Duplicated dashboard: %v", dashboardGroup.Name)
		} else {
			groupSet.Insert(dashboardGroup.Name)
		}

		if dashboardSet.Has(dashboardGroup.Name) {
			t.Errorf("%v is both a dashboard and dashboard group name.", dashboardGroup.Name)
		}

		for _, dashboard := range dashboardGroup.DashboardNames {
			// All dashboard must not have duplicated names
			if assignedGroup, ok := dashboardToGroupMap[dashboard]; ok {
				t.Errorf("Duplicated dashboard %v in dashboard group %v and %v", dashboard, assignedGroup, dashboardGroup.Name)
			} else {
				dashboardToGroupMap[dashboard] = dashboardGroup.Name
			}

			if !dashboardSet.Has(dashboard) {
				t.Errorf("Dashboard %v needs to be defined before adding to a dashboard group!", dashboard)
			}

			if !strings.HasPrefix(dashboard, dashboardGroup.Name) {
				t.Errorf("Dashboard %v in group %v must have the group name as a prefix", dashboard, dashboardGroup.Name)
			}
		}
	}

	// Dashboards that match this dashboard group's prefix should be a part of it, unless this group is the prefix of the assigned group
	// (e.g. knative and knative-sandbox).
	for thisGroup := range groupSet {
		for dashboard := range dashboardSet {
			if strings.HasPrefix(dashboard, thisGroup+"-") {
				assignedGroup, ok := dashboardToGroupMap[dashboard]
				if !ok {
					t.Errorf("Dashboard %v should be in dashboard_group %v", dashboard, thisGroup)
				} else if assignedGroup != thisGroup && !strings.HasPrefix(assignedGroup, thisGroup) {
					t.Errorf("Dashboard %v should be in dashboard_group %v instead of dashboard_group %v", dashboard, thisGroup, assignedGroup)
				}
			}
		}
	}

	// All Testgroup should be mapped to one or more tabs
	missedTestgroups := false
	for testgroupname, occurrence := range testgroupMap {
		if occurrence == 1 {
			t.Errorf("Testgroup %v - defined but not used in any dashboards.", testgroupname)
			missedTestgroups = true
		}
	}
	if missedTestgroups {
		t.Logf("Note: Testgroups are automatically defined for postsubmits and periodics.")
		t.Logf("Testgroups can be added to a dashboard either by using the `testgrid-dashboards` annotation on the prowjob, or by adding them to testgrid/config.yaml")
	}
}

// TODO: These are all repos that don't have their presubmits in testgrid.
// Convince sig leads or subproject owners this is a bad idea and whittle this down
// to just kubernetes-security/
// Tracking issue: https://github.com/kubernetes/test-infra/issues/18159
var noPresubmitsInTestgridPrefixes = []string{
	"containerd/cri",
	"kubernetes-sigs/cluster-capacity",
	"kubernetes-sigs/gcp-filestore-csi-driver",
	"kubernetes-sigs/kind",
	"kubernetes-sigs/kubetest2",
	"kubernetes-sigs/oci-proxy",
	"kubernetes-sigs/kubebuilder-declarative-pattern",
	"kubernetes-sigs/scheduler-plugins",
	"kubernetes-sigs/service-catalog",
	"kubernetes-sigs/sig-storage-local-static-provisioner",
	"kubernetes-sigs/slack-infra",
	"kubernetes-sigs/testing_frameworks",
	"kubernetes/client-go",
	"kubernetes/cloud-provider-openstack",
	"kubernetes/dns",
	"kubernetes/enhancements",
	"kubernetes/ingress-gce",
	"kubernetes/kubeadm",
	"kubernetes/minikube",
	// This is the one entry that should be here
	"kubernetes-security/",
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// A job is merge-blocking if it:
// - is not optional
// - reports (aka does not skip reporting)
// - always runs OR runs if some path changed
func isMergeBlocking(job prow_config.Presubmit) bool {
	return !job.Optional && !job.SkipReport && (job.AlwaysRun || job.RunIfChanged != "" || job.SkipIfOnlyChanged != "")
}

// All jobs in presubmits-kubernetes-blocking must be merge-blocking for kubernetes/kubernetes
// All jobs that are merge-blocking for kubernetes/kubernetes must be in presubmits-kubernetes-blocking
func TestPresubmitsKubernetesDashboards(t *testing.T) {
	var dashboard *config_pb.Dashboard
	repo := "kubernetes/kubernetes"
	dash := "presubmits-kubernetes-blocking"
	for _, d := range cfg.Dashboards {
		if d.Name == dash {
			dashboard = d
		}
	}
	if dashboard == nil {
		t.Fatalf("Missing dashboard: %s", dash)
	}
	testgroups := make(map[string]bool)
	for _, tab := range dashboard.DashboardTab {
		testgroups[tab.TestGroupName] = false
	}
	jobs := make(map[string]bool)
	for _, job := range prowConfig.AllStaticPresubmits([]string{repo}) {
		if isMergeBlocking(job) {
			jobs[job.Name] = false
		}
	}
	for job, seen := range jobs {
		if _, ok := testgroups[job]; !seen && !ok {
			t.Errorf("%s: job is merge-blocking for %s but missing from %s", job, repo, dash)
		}
		jobs[job] = true
	}
	for tg, seen := range testgroups {
		if _, ok := jobs[tg]; !seen && !ok {
			t.Errorf("%s: should not be in %s because not actually merge-blocking for %s", tg, dash, repo)
		}
		testgroups[tg] = true
	}
}

func TestKubernetesProwInstanceJobsMustHaveMatchingTestgridEntries(t *testing.T) {
	jobs := make(map[string]bool)

	for repo, presubmits := range prowConfig.PresubmitsStatic {
		// Assume that all jobs in the exceptionList are valid
		if hasAnyPrefix(repo, noPresubmitsInTestgridPrefixes) {
			for _, job := range presubmits {
				jobs[job.Name] = true
			}
			continue
		}
		for _, job := range presubmits {
			jobs[job.Name] = false
		}
	}

	for _, job := range prowConfig.AllStaticPostsubmits([]string{}) {
		jobs[job.Name] = false
	}

	for _, job := range prowConfig.AllPeriodics() {
		jobs[job.Name] = false
	}

	// Ignore any test groups that get their results from a gcs prefix
	// that is not populated by the kubernetes prow instance
	testgroups := make(map[string]bool)
	for _, testgroup := range cfg.TestGroups {
		for _, prowGcsPrefix := range prowGcsPrefixes {
			if strings.Contains(testgroup.GcsPrefix, prowGcsPrefix) {
				// The convention is that the job name is the final part of the GcsPrefix
				job := filepath.Base(testgroup.GcsPrefix)
				testgroups[job] = false
				break // to next testgroup
			}
		}
	}

	// Each job running in the kubernetes prow instance must have an
	// identically named test_groups entry in the kubernetes testgrid config
	for job := range jobs {
		if _, ok := testgroups[job]; ok {
			testgroups[job] = true
			jobs[job] = true
		}
	}

	// Conclusion
	for job, valid := range jobs {
		if !valid {
			t.Errorf("Job %v does not have a matching testgrid testgroup", job)
		}
	}

	for testgroup, valid := range testgroups {
		if !valid {
			t.Errorf("Testgrid group %v is supposed to be moved to have their presubmits in testgrid. See this issue: https://github.com/kubernetes/test-infra/issues/18159", testgroup)
		}
	}
}

func TestReleaseBlockingJobsMustHaveTestgridDescriptions(t *testing.T) {
	// TODO(spiffxp): start with master, enforce for all release branches
	re := regexp.MustCompile("^sig-release-master-(blocking|informing)$")
	for _, dashboard := range cfg.Dashboards {
		if !re.MatchString(dashboard.Name) {
			continue
		}
		suffix := re.FindStringSubmatch(dashboard.Name)[1]
		for _, dashboardtab := range dashboard.DashboardTab {
			intro := fmt.Sprintf("dashboard_tab %v/%v is release-%v", dashboard.Name, dashboardtab.Name, suffix)
			if dashboardtab.Name == "" {
				t.Errorf("%v: - Must have a name", intro)
			}
			if dashboardtab.TestGroupName == "" {
				t.Errorf("%v: - Must have a test_group_name", intro)
			}
			if dashboardtab.Description == "" {
				t.Errorf("%v: - Must have a description", intro)
			}

			// TODO(spiffxp): enforce for informing as well
			if suffix == "informing" {
				// if !strings.HasPrefix(dashboardtab.Description, "OWNER: ") {
				// 	t.Logf("NOTICE: %v: - Must have a description that starts with OWNER: ", intro)
				// }
				// if dashboardtab.AlertOptions == nil {
				// 	t.Logf("NOTICE: %v: - Must have alert_options (ensure informing dashboard is listed first in testgrid-dashboards)", intro)
				// } else if dashboardtab.AlertOptions.AlertMailToAddresses == "" {
				// 	t.Logf("NOTICE: %v: - Must have alert_options.alert_mail_to_addresses", intro)
				// }
			} else {
				// if dashboardtab.AlertOptions != nil {
				// 	t.Log("------", dashboardtab.Name, dashboardtab.AlertOptions.AlertMailToAddresses)
				// }
				if dashboardtab.Name == "gci-gce-ingress" {
					t.Log("------------------------------", dashboardtab.AlertOptions.AlertMailToAddresses)
				}
				if dashboardtab.AlertOptions == nil {
					t.Errorf("%v: - Must have alert_options (ensure blocking dashboard is listed first in testgrid-dashboards)", intro)
				} else if dashboardtab.AlertOptions.AlertMailToAddresses == "" {

					t.Errorf("%v: - Must have alert_options.alert_mail_to_addresses", intro)
				}
			}
		}
	}
}

func TestNoEmpyMailToAddresses(t *testing.T) {
	for _, dashboard := range cfg.Dashboards {
		for _, dashboardtab := range dashboard.DashboardTab {
			intro := fmt.Sprintf("dashboard_tab %v/%v", dashboard.Name, dashboardtab.Name)
			if dashboardtab.AlertOptions != nil {
				if dashboardtab.AlertOptions.AlertMailToAddresses == "" {
					continue
				}
				mails := strings.Split(dashboardtab.AlertOptions.AlertMailToAddresses, ",")
				for _, m := range mails {
					_, err := mail.ParseAddress(m)
					if err != nil {
						t.Errorf("%v: - invalid alert_mail_to_address '%v': %v", intro, m, err)
					}
				}
			}
		}
	}
}
