/*
Copyright 2026 The Kubernetes Authors.

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

package generate_test

import (
	"reflect"
	"strings"
	"testing"

	"k8s.io/test-infra/releng/generate-tests/internal/config"
	"k8s.io/test-infra/releng/generate-tests/internal/generate"
	"k8s.io/test-infra/releng/generate-tests/internal/types"
	"k8s.io/test-infra/releng/generate-tests/internal/version"
)

func singleTier() []version.Tier {
	return []version.Tier{
		{Marker: "beta", Version: version.Version{Major: 1, Minor: 35}, Interval: "1h"},
	}
}

func allTiers() []version.Tier {
	return []version.Tier{
		{Marker: "beta", Version: version.Version{Major: 1, Minor: 35}, Interval: "1h"},
		{Marker: "stable1", Version: version.Version{Major: 1, Minor: 34}, Interval: "2h"},
		{Marker: "stable2", Version: version.Version{Major: 1, Minor: 33}, Interval: "6h"},
		{Marker: "stable3", Version: version.Version{Major: 1, Minor: 32}, Interval: "24h"},
		{Marker: "stable4", Version: version.Version{Major: 1, Minor: 31}, Interval: "24h"},
	}
}

func TestGenerateJobs(t *testing.T) {
	t.Parallel()

	prowConfig, tgc := generate.Jobs(allTiers())

	if len(prowConfig.Periodics) != 25 {
		t.Fatalf("expected 25 jobs, got %d", len(prowConfig.Periodics))
	}

	if len(tgc.TestGroups) != 25 {
		t.Fatalf("expected 25 test groups, got %d", len(tgc.TestGroups))
	}

	// Verify jobs are sorted by name.
	for idx := 1; idx < len(prowConfig.Periodics); idx++ {
		if prowConfig.Periodics[idx].Name < prowConfig.Periodics[idx-1].Name {
			t.Errorf(
				"jobs not sorted: %q < %q",
				prowConfig.Periodics[idx].Name,
				prowConfig.Periodics[idx-1].Name,
			)
		}
	}
}

func TestJobNaming(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	expectedNames := []string{
		"ci-kubernetes-e2e-gce-cos-k8sbeta-alphafeatures",
		"ci-kubernetes-e2e-gce-cos-k8sbeta-default",
		"ci-kubernetes-e2e-gce-cos-k8sbeta-reboot",
		"ci-kubernetes-e2e-gce-cos-k8sbeta-serial",
		"ci-kubernetes-e2e-gce-cos-k8sbeta-slow",
	}

	if len(prowConfig.Periodics) != len(expectedNames) {
		t.Fatalf("expected %d jobs, got %d", len(expectedNames), len(prowConfig.Periodics))
	}

	for idx, name := range expectedNames {
		if prowConfig.Periodics[idx].Name != name {
			t.Errorf("job[%d] name: got %q, want %q", idx, prowConfig.Periodics[idx].Name, name)
		}
	}
}

func TestBlockingJobAnnotations(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	for _, job := range prowConfig.Periodics {
		dashboard := job.Annotations["testgrid-dashboards"]
		switch {
		case strings.HasSuffix(job.Name, "-alphafeatures"),
			strings.HasSuffix(job.Name, "-default"),
			strings.HasSuffix(job.Name, "-reboot"):
			if !strings.Contains(dashboard, "blocking") {
				t.Errorf("%s: expected blocking dashboard, got %q", job.Name, dashboard)
			}
		case strings.HasSuffix(job.Name, "-serial"),
			strings.HasSuffix(job.Name, "-slow"):
			if !strings.Contains(dashboard, "informing") {
				t.Errorf("%s: expected informing dashboard, got %q", job.Name, dashboard)
			}

			if job.Annotations["testgrid-num-failures-to-alert"] != "6" {
				t.Errorf("%s: expected testgrid-num-failures-to-alert=6, got %q",
					job.Name, job.Annotations["testgrid-num-failures-to-alert"])
			}
		}
	}
}

func TestStable4DefaultNumFailures(t *testing.T) {
	t.Parallel()

	tiers := []version.Tier{
		{Marker: "stable4", Version: version.Version{Major: 1, Minor: 31}, Interval: "24h"},
	}

	prowConfig, _ := generate.Jobs(tiers)

	for _, job := range prowConfig.Periodics {
		if strings.HasSuffix(job.Name, "-default") {
			if job.Annotations["testgrid-num-failures-to-alert"] != "6" {
				t.Errorf(
					"%s: expected testgrid-num-failures-to-alert=6 for stable4-default, got %q",
					job.Name,
					job.Annotations["testgrid-num-failures-to-alert"],
				)
			}
		}
	}
}

func TestJobIntervals(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(allTiers())

	intervalByMarker := map[string]string{
		"beta": "1h", "stable1": "2h", "stable2": "6h",
		"stable3": "24h", "stable4": "24h",
	}

	for _, job := range prowConfig.Periodics {
		for marker, interval := range intervalByMarker {
			if strings.Contains(job.Name, "k8s"+marker+"-") {
				if job.Interval != interval {
					t.Errorf("%s: expected interval %q, got %q", job.Name, interval, job.Interval)
				}
			}
		}
	}
}

func TestJobArgs(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	for _, job := range prowConfig.Periodics {
		args := job.Spec.Containers[0].Args

		if args[0] != "--check-leaked-resources" {
			t.Errorf("%s: first arg should be --check-leaked-resources, got %q", job.Name, args[0])
		}

		if args[1] != "--provider=gce" {
			t.Errorf("%s: second arg should be --provider=gce, got %q", job.Name, args[1])
		}

		if args[2] != "--gcp-region=us-central1" {
			t.Errorf("%s: third arg should be --gcp-region=us-central1, got %q", job.Name, args[2])
		}

		if args[3] != "--gcp-node-image=gci" {
			t.Errorf("%s: fourth arg should be --gcp-node-image=gci, got %q", job.Name, args[3])
		}

		if args[4] != "--extract=ci/latest-1.35" {
			t.Errorf("%s: fifth arg should be --extract=ci/latest-1.35, got %q", job.Name, args[4])
		}
	}
}

func TestSerialJobOverride(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	for _, job := range prowConfig.Periodics {
		if !strings.HasSuffix(job.Name, "-serial") {
			continue
		}

		args := job.Spec.Containers[0].Args
		found := false

		for _, arg := range args {
			if strings.Contains(arg, "sig-cloud-provider-gcp") {
				found = true

				break
			}
		}

		if !found {
			t.Errorf("%s: expected sig-cloud-provider-gcp in skip pattern", job.Name)
		}
	}
}

func TestExtraRefs(t *testing.T) {
	t.Parallel()

	tiers := []version.Tier{
		{Marker: "beta", Version: version.Version{Major: 1, Minor: 35}, Interval: "1h"},
		{Marker: "stable1", Version: version.Version{Major: 1, Minor: 34}, Interval: "2h"},
	}

	prowConfig, _ := generate.Jobs(tiers)

	for _, job := range prowConfig.Periodics {
		if len(job.ExtraRefs) != 1 {
			t.Errorf("%s: expected 1 extra_ref, got %d", job.Name, len(job.ExtraRefs))

			continue
		}

		ref := job.ExtraRefs[0]
		if ref.Org != "kubernetes" || ref.Repo != "kubernetes" {
			t.Errorf("%s: expected kubernetes/kubernetes, got %s/%s", job.Name, ref.Org, ref.Repo)
		}

		if ref.PathAlias != "k8s.io/kubernetes" {
			t.Errorf("%s: expected path_alias k8s.io/kubernetes, got %q", job.Name, ref.PathAlias)
		}

		if strings.Contains(job.Name, "k8sbeta-") {
			if ref.BaseRef != "release-1.35" {
				t.Errorf("%s: expected base_ref release-1.35, got %q", job.Name, ref.BaseRef)
			}
		} else if strings.Contains(job.Name, "k8sstable1-") {
			if ref.BaseRef != "release-1.34" {
				t.Errorf("%s: expected base_ref release-1.34, got %q", job.Name, ref.BaseRef)
			}
		}
	}
}

func TestTestGridConfig(t *testing.T) {
	t.Parallel()

	_, tgc := generate.Jobs(singleTier())

	if len(tgc.TestGroups) != 5 {
		t.Fatalf("expected 5 test groups, got %d", len(tgc.TestGroups))
	}

	for _, testGroup := range tgc.TestGroups {
		expectedPrefix := config.GCSLogPrefix + testGroup.Name
		if testGroup.GCSPrefix != expectedPrefix {
			t.Errorf(
				"%s: expected gcs_prefix %q, got %q",
				testGroup.Name, expectedPrefix, testGroup.GCSPrefix,
			)
		}

		if len(testGroup.ColumnHeader) != 4 {
			t.Errorf(
				"%s: expected 4 column headers, got %d",
				testGroup.Name, len(testGroup.ColumnHeader),
			)
		}
	}
}

func TestDecorationTimeout(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	expectedTimeouts := map[string]string{
		"ci-kubernetes-e2e-gce-cos-k8sbeta-alphafeatures": "200m",
		"ci-kubernetes-e2e-gce-cos-k8sbeta-default":       "140m",
		"ci-kubernetes-e2e-gce-cos-k8sbeta-reboot":        "200m",
		"ci-kubernetes-e2e-gce-cos-k8sbeta-serial":        "680m",
		"ci-kubernetes-e2e-gce-cos-k8sbeta-slow":          "170m",
	}

	for _, job := range prowConfig.Periodics {
		expected, ok := expectedTimeouts[job.Name]
		if !ok {
			continue
		}

		if job.DecorationConfig["timeout"] != expected {
			t.Errorf(
				"%s: expected timeout %q, got %q",
				job.Name,
				expected,
				job.DecorationConfig["timeout"],
			)
		}
	}
}

func TestApplyOverrides(t *testing.T) {
	t.Parallel()

	args := []string{"--timeout=100m", "--test_args=original", "--ginkgo-parallel=1"}
	overrides := []string{"--test_args=overridden"}

	result := generate.ApplyOverrides(args, overrides)

	expected := []string{"--timeout=100m", "--ginkgo-parallel=1", "--test_args=overridden"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(result), result)
	}

	for idx, exp := range expected {
		if result[idx] != exp {
			t.Errorf("arg[%d]: got %q, want %q", idx, result[idx], exp)
		}
	}
}

func TestApplyOverridesNoMatch(t *testing.T) {
	t.Parallel()

	args := []string{"--timeout=100m", "--ginkgo-parallel=1"}
	overrides := []string{"--new-flag=value"}

	result := generate.ApplyOverrides(args, overrides)

	expected := []string{"--timeout=100m", "--ginkgo-parallel=1", "--new-flag=value"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestApplyOverridesEmpty(t *testing.T) {
	t.Parallel()

	args := []string{"--timeout=100m", "--ginkgo-parallel=1"}

	result := generate.ApplyOverrides(args, nil)
	if !reflect.DeepEqual(result, args) {
		t.Errorf("got %v, want %v", result, args)
	}
}

func TestApplyOverridesMultiple(t *testing.T) {
	t.Parallel()

	args := []string{"--timeout=100m", "--test_args=original", "--ginkgo-parallel=1"}
	overrides := []string{"--test_args=overridden", "--ginkgo-parallel=30"}

	result := generate.ApplyOverrides(args, overrides)

	expected := []string{"--timeout=100m", "--test_args=overridden", "--ginkgo-parallel=30"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestGenerateJobsDeterministic(t *testing.T) {
	t.Parallel()

	tiers := allTiers()

	prowConfig1, tgc1 := generate.Jobs(tiers)
	prowConfig2, tgc2 := generate.Jobs(tiers)

	if len(prowConfig1.Periodics) != len(prowConfig2.Periodics) {
		t.Fatal("different number of jobs on second run")
	}

	for idx := range prowConfig1.Periodics {
		if prowConfig1.Periodics[idx].Name != prowConfig2.Periodics[idx].Name {
			t.Errorf(
				"job[%d] name differs: %q vs %q",
				idx,
				prowConfig1.Periodics[idx].Name,
				prowConfig2.Periodics[idx].Name,
			)
		}
	}

	if len(tgc1.TestGroups) != len(tgc2.TestGroups) {
		t.Fatal("different number of test groups on second run")
	}

	for idx := range tgc1.TestGroups {
		if tgc1.TestGroups[idx].Name != tgc2.TestGroups[idx].Name {
			t.Errorf(
				"tg[%d] name differs: %q vs %q",
				idx,
				tgc1.TestGroups[idx].Name,
				tgc2.TestGroups[idx].Name,
			)
		}
	}
}

func verifyResourcesEqual(t *testing.T, job string, res types.ProwResources) {
	t.Helper()

	if res.Requests.CPU != res.Limits.CPU {
		t.Errorf("%s: CPU requests %q != limits %q", job, res.Requests.CPU, res.Limits.CPU)
	}

	if res.Requests.Memory != res.Limits.Memory {
		t.Errorf("%s: memory requests %q != limits %q", job, res.Requests.Memory, res.Limits.Memory)
	}
}

func TestJobResources(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	for _, job := range prowConfig.Periodics {
		res := job.Spec.Containers[0].Resources
		verifyResourcesEqual(t, job.Name, res)

		switch {
		case strings.HasSuffix(job.Name, "-default"):
			verifyResourceValues(t, job.Name, res, "2000m", "6Gi")
		case strings.HasSuffix(job.Name, "-slow"):
			verifyResourceValues(t, job.Name, res, "1000m", "6Gi")
		default:
			verifyResourceValues(t, job.Name, res, "1000m", "3Gi")
		}
	}
}

func verifyResourceValues(
	t *testing.T, job string, res types.ProwResources, cpu, memory string,
) {
	t.Helper()

	if res.Requests.CPU != cpu {
		t.Errorf("%s: expected CPU %s, got %q", job, cpu, res.Requests.CPU)
	}

	if res.Requests.Memory != memory {
		t.Errorf("%s: expected memory %s, got %q", job, memory, res.Requests.Memory)
	}
}

func TestJobLabels(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	for _, job := range prowConfig.Periodics {
		if job.Labels["preset-service-account"] != "true" {
			t.Errorf("%s: missing preset-service-account label", job.Name)
		}

		if job.Labels["preset-k8s-ssh"] != "true" {
			t.Errorf("%s: missing preset-k8s-ssh label", job.Name)
		}
	}
}

func TestJobCluster(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	for _, job := range prowConfig.Periodics {
		if job.Cluster != config.Cluster {
			t.Errorf("%s: expected cluster %q, got %q", job.Name, config.Cluster, job.Cluster)
		}
	}
}

func TestJobDecorate(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	for _, job := range prowConfig.Periodics {
		if !job.Decorate {
			t.Errorf("%s: expected decorate=true", job.Name)
		}
	}
}

func TestJobTags(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	for _, job := range prowConfig.Periodics {
		if len(job.Tags) != 1 || job.Tags[0] != "generated" {
			t.Errorf("%s: expected tags=[generated], got %v", job.Name, job.Tags)
		}
	}
}

func TestJobCommand(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	for _, job := range prowConfig.Periodics {
		cmd := job.Spec.Containers[0].Command
		if len(cmd) != 2 || cmd[0] != "runner.sh" ||
			cmd[1] != "/workspace/scenarios/kubernetes_e2e.py" {
			t.Errorf(
				"%s: expected command [runner.sh, /workspace/scenarios/kubernetes_e2e.py], got %v",
				job.Name,
				cmd,
			)
		}
	}
}

func TestJobImage(t *testing.T) {
	t.Parallel()

	prowConfig, _ := generate.Jobs(singleTier())

	for _, job := range prowConfig.Periodics {
		if job.Spec.Containers[0].Image != config.DefaultImage {
			t.Errorf(
				"%s: expected image %q, got %q",
				job.Name,
				config.DefaultImage,
				job.Spec.Containers[0].Image,
			)
		}
	}
}

func TestTestGridGCSPrefix(t *testing.T) {
	t.Parallel()

	_, tgc := generate.Jobs(singleTier())

	for _, testGroup := range tgc.TestGroups {
		expected := config.GCSLogPrefix + testGroup.Name
		if testGroup.GCSPrefix != expected {
			t.Errorf(
				"%s: expected gcs_prefix %q, got %q",
				testGroup.Name, expected, testGroup.GCSPrefix,
			)
		}
	}
}

func TestTestGridColumnHeaders(t *testing.T) {
	t.Parallel()

	_, tgc := generate.Jobs(singleTier())

	expectedHeaders := []string{"node_os_image", "master_os_image", "Commit", "infra-commit"}

	for _, testGroup := range tgc.TestGroups {
		if len(testGroup.ColumnHeader) != len(expectedHeaders) {
			t.Errorf(
				"%s: expected %d column headers, got %d",
				testGroup.Name,
				len(expectedHeaders),
				len(testGroup.ColumnHeader),
			)

			continue
		}

		for idx, eh := range expectedHeaders {
			if testGroup.ColumnHeader[idx].ConfigurationValue != eh {
				t.Errorf("%s: column_header[%d]: got %q, want %q",
					testGroup.Name, idx, testGroup.ColumnHeader[idx].ConfigurationValue, eh)
			}
		}
	}
}
