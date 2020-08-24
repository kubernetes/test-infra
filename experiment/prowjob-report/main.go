/*
Copyright 2018 The Kubernetes Authors.

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

// This generates a csv listing of all of our prowjobs to import into a
// spreadsheet so humans could see prowjob info relevant to enforcing
// policies at a glance

// The intent is for actual tests to enforce these policies, but their
// output is not amenable to generating a report that could be broadcast
// to humans

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	// pjv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	cfg "k8s.io/test-infra/prow/config"
)

// TODO: parse testgrid config to catch
//	- jobs that aren't prowjobs but are on release-informing dashboards
//	- jobs that don't declare testgrid info via annotations
var configPath = flag.String("config", "../../config/prow/config.yaml", "Path to prow config")
var jobConfigPath = flag.String("job-config", "../../config/jobs", "Path to prow job config")
var reportFormat = flag.String("format", "csv", "Output format [csv|json|html] defaults to csv")
var reportDate = flag.String("date", "now", "Date to include in report ('now' is converted to today)")

// Loaded at TestMain.
var prowConfig *cfg.Config

func main() {
	flag.Parse()
	if *configPath == "" {
		fmt.Println("--config must set")
		os.Exit(1)
	}
	if *jobConfigPath == "" {
		fmt.Println("--job-config must set")
		os.Exit(1)
	}

	conf, err := cfg.Load(*configPath, *jobConfigPath)
	if err != nil {
		fmt.Printf("configPath: %v\n", *configPath)
		fmt.Printf("jobConfigPath: %v\n", *jobConfigPath)
		fmt.Printf("Could not load config: %v\n", err)
		os.Exit(1)
	}
	prowConfig = conf

	date := *reportDate
	if date == "now" {
		date = time.Now().Format("2006-01-02")
	}

	rows := GatherProwJobReportRows(date)
	switch *reportFormat {
	case "csv":
		PrintCSVReport(rows)
	case "json":
		PrintJSONReport(rows)
	case "html":
		PrintHTMLReport(rows)
	default:
		fmt.Printf("ERROR: unknown format: %v\n", *reportFormat)
	}
}

// Consistently sorted ProwJob configs

func sortedPeriodics() []cfg.Periodic {
	jobs := prowConfig.AllPeriodics()
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Name < jobs[j].Name
	})
	return jobs
}

func sortedPresubmitsByRepo() (repos []string, jobsByRepo map[string][]cfg.Presubmit) {
	jobsByRepo = make(map[string][]cfg.Presubmit)
	for repo, jobs := range prowConfig.PresubmitsStatic {
		repos = append(repos, repo)
		sort.Slice(jobs, func(i, j int) bool {
			return jobs[i].Name < jobs[j].Name
		})
		jobsByRepo[repo] = jobs
	}
	sort.Strings(repos)
	return repos, jobsByRepo
}

func sortedPostsubmitsByRepo() (repos []string, jobsByRepo map[string][]cfg.Postsubmit) {
	jobsByRepo = make(map[string][]cfg.Postsubmit)
	for repo, jobs := range prowConfig.PostsubmitsStatic {
		repos = append(repos, repo)
		sort.Slice(jobs, func(i, j int) bool {
			return jobs[i].Name < jobs[j].Name
		})
		jobsByRepo[repo] = jobs
	}
	sort.Strings(repos)
	return repos, jobsByRepo
}

// ResourceRequirement utils

func TotalResourceRequirements(spec *corev1.PodSpec) corev1.ResourceRequirements {
	resourceNames := []corev1.ResourceName{
		corev1.ResourceCPU,
		corev1.ResourceMemory,
	}
	total := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}
	zero := resource.MustParse("0")
	for _, r := range resourceNames {
		total.Requests[r] = zero.DeepCopy()
		total.Limits[r] = zero.DeepCopy()
	}
	if spec == nil {
		return total
	}
	for _, c := range spec.Containers {
		for _, r := range resourceNames {
			if limit, ok := c.Resources.Limits[r]; ok {
				tmp := total.Limits[r]
				tmp.Add(limit)
				total.Limits[r] = tmp
			}
			if request, ok := c.Resources.Requests[r]; ok {
				tmp := total.Requests[r]
				tmp.Add(request)
				total.Requests[r] = tmp
			}
		}
	}
	return total
}

func ScaledValue(q resource.Quantity, s resource.Scale) int64 {
	return q.ScaledValue(s)
}

// Testgrid dashboard utils

// Primary dashboard aka which is most likely to have more viewers
// Choose from: sig-release-.*, or sig-.*, or first in list
func PrimaryDashboard(job cfg.JobBase) string {
	dashboardsAnnotation, ok := job.Annotations["testgrid-dashboards"]
	if !ok {
		// technically it could be specified in a testgrid config, would need testgrid/cmd/configurator code to know for sure
		return "TODO"
	}
	dashboards := []string{}
	for _, db := range strings.Split(dashboardsAnnotation, ",") {
		dashboards = append(dashboards, strings.TrimSpace(db))
	}
	for _, db := range dashboards {
		if strings.HasPrefix(db, "sig-release-") {
			return db
		}
	}
	for _, db := range dashboards {
		if strings.HasPrefix(db, "sig-") {
			return db
		}
	}
	return dashboards[0]
}

// Owner dashboard aka who is responsible for maintaining the job
// Choose from: sig-(not-release)-*, or sig-release, or first in list
func OwnerDashboard(job cfg.JobBase) string {
	dashboardsAnnotation, ok := job.Annotations["testgrid-dashboards"]
	if !ok {
		// technically it could be specified in a testgrid config, would need testgrid/cmd/configurator code to know for sure
		return "TODO"
	}
	dashboards := []string{}
	for _, db := range strings.Split(dashboardsAnnotation, ",") {
		dashboards = append(dashboards, strings.TrimSpace(db))
	}
	for _, db := range dashboards {
		if strings.HasPrefix(db, "sig-") && !strings.HasPrefix(db, "sig-release-") {
			return db
		}
	}
	for _, db := range dashboards {
		if strings.HasPrefix(db, "sig-release-") {
			return db
		}
	}
	return dashboards[0]
}

func verifyPodQOSGuaranteed(spec *corev1.PodSpec) (errs []error) {
	resourceNames := []corev1.ResourceName{
		corev1.ResourceCPU,
		corev1.ResourceMemory,
	}
	zero := resource.MustParse("0")
	for _, c := range spec.Containers {
		for _, r := range resourceNames {
			limit, ok := c.Resources.Limits[r]
			if !ok {
				errs = append(errs, fmt.Errorf("container '%v' should have resources.limits[%v] specified", c.Name, r))
			}
			request, ok := c.Resources.Requests[r]
			if !ok {
				errs = append(errs, fmt.Errorf("container '%v' should have resources.requests[%v] specified", c.Name, r))
			}
			if limit.Cmp(zero) == 0 {
				errs = append(errs, fmt.Errorf("container '%v' resources.limits[%v] should be non-zero", c.Name, r))
			} else if limit.Cmp(request) != 0 {
				errs = append(errs, fmt.Errorf("container '%v' resources.limits[%v] (%v) should match request (%v)", c.Name, r, limit.String(), request.String()))
			}
		}
	}
	return errs
}

// A PodSpec is PodQOS Guaranteed if all of its containers have non-zero
// resource limits equal to their resource requests for cpu and memory
func isPodQOSGuaranteed(spec *corev1.PodSpec) bool {
	return len(verifyPodQOSGuaranteed(spec)) == 0
}

// A presubmit is merge-blocking if it:
// - is not optional
// - reports (aka does not skip reporting)
// - always runs OR runs if some path changed
func isMergeBlocking(job cfg.Presubmit) bool {
	return !job.Optional && !job.SkipReport && (job.AlwaysRun || job.RunIfChanged != "")
}

func guessPeriodicRepoAndBranch(job cfg.Periodic) (repo, branch string) {
	repo = "TODO"
	branch = "TODO"
	defaultBranch := "master"
	// First, assume the first extra ref is our repo
	if len(job.ExtraRefs) > 0 {
		ref := job.ExtraRefs[0]
		repo = fmt.Sprintf("%s/%s", ref.Org, ref.Repo)
		branch = ref.BaseRef
		return
	}

	// If we have no extra refs, maybe we're using the defunct bootstrap args,
	// in which case we assume the job is a single-container pod, and then...

	// Assume the first repo arg we find is "the" repo; save scenario for later
	scenario := ""
	for _, arg := range job.Spec.Containers[0].Args {
		if strings.HasPrefix(arg, "--scenario=") {
			scenario = strings.Split(arg, "=")[1]
		}
		if !strings.HasPrefix(arg, "--repo=") {
			continue
		}
		arg = strings.SplitN(arg, "=", 2)[1]
		arg = strings.ReplaceAll(arg, "sigs.k8s.io", "kubernetes-sigs")
		arg = strings.ReplaceAll(arg, "k8s.io", "kubernetes")
		arg = strings.ReplaceAll(arg, "github.com/", "")
		split := strings.Split(arg, "=")
		repo = split[0]
		branch = defaultBranch
		if len(split) > 1 {
			branch = split[1]
		}
		return
	}

	// We didn't find an explicit repo, so now assume if --scenario=kubernetes_e2e
	// was used, the repo is kubernetes/kubernetes
	if scenario == "kubernetes_e2e" {
		repo = "kubernetes/kubernetes"
		branch = defaultBranch
	}
	return
}

type ProwJobReportRow struct {
	Date             string // TODO: make this an actual date instead a string pass-through
	Name             string
	ProwJobType      string
	Repo             string
	Branch           string
	PrimaryDashboard string
	OwnerDashboard   string
	Cluster          string
	MaxConcurrency   int
	AlwaysRun        bool // presubmits may be false
	MergeBlocking    bool // presubmits may be true
	QOSGuaranteed    bool
	RequestMilliCPU  int64
	LimitMilliCPU    int64
	RequestGigaMem   int64
	LimitGigaMem     int64
}

func NewProwJobReportRow(date, jobType, repo, branch string, alwaysRun, mergeBlocking bool, job cfg.JobBase) ProwJobReportRow {
	r := TotalResourceRequirements(job.Spec)
	// TODO: actually read testgrid config please
	primaryDashboard := PrimaryDashboard(job)
	if mergeBlocking && repo == "kubernetes/kubernetes" {
		primaryDashboard = "kubernetes-presubmits-blocking"
	}
	return ProwJobReportRow{
		Date:             date,
		Name:             job.Name,
		ProwJobType:      jobType,
		Repo:             repo,
		Branch:           branch,
		PrimaryDashboard: primaryDashboard,
		OwnerDashboard:   OwnerDashboard(job),
		Cluster:          job.Cluster,
		MaxConcurrency:   job.MaxConcurrency,
		AlwaysRun:        alwaysRun,
		MergeBlocking:    mergeBlocking,
		QOSGuaranteed:    isPodQOSGuaranteed(job.Spec),
		RequestMilliCPU:  ScaledValue(r.Requests[corev1.ResourceCPU], resource.Milli),
		LimitMilliCPU:    ScaledValue(r.Limits[corev1.ResourceCPU], resource.Milli),
		RequestGigaMem:   ScaledValue(r.Requests[corev1.ResourceMemory], resource.Giga),
		LimitGigaMem:     ScaledValue(r.Limits[corev1.ResourceMemory], resource.Giga),
	}
}

func GatherProwJobReportRows(date string) []ProwJobReportRow {
	rows := []ProwJobReportRow{}
	for _, job := range sortedPeriodics() {
		// TODO: depending on whether decoration or bootstrap is used repo could be any number of repos
		repo, branch := guessPeriodicRepoAndBranch(job)
		rows = append(rows, NewProwJobReportRow(date, "periodic", repo, branch, true, false, job.JobBase))
	}
	repos, postsubmitsByRepo := sortedPostsubmitsByRepo()
	for _, repo := range repos {
		for _, job := range postsubmitsByRepo[repo] {
			branch := "default"
			if len(job.Branches) > 0 {
				branch = strings.Join(job.Branches, "|")
			}
			rows = append(rows, NewProwJobReportRow(date, "postsubmit", repo, branch, false, false, job.JobBase))
		}
	}
	repos, presubmitsByRepo := sortedPresubmitsByRepo()
	for _, repo := range repos {
		for _, job := range presubmitsByRepo[repo] {
			branch := "default"
			if len(job.Branches) > 0 {
				branch = strings.Join(job.Branches, "|")
			}
			rows = append(rows, NewProwJobReportRow(date, "presubmit", repo, branch, job.AlwaysRun, isMergeBlocking(job), job.JobBase))
		}
	}
	return rows
}

func PrintCSVReport(rows []ProwJobReportRow) {
	fmt.Printf("report_date, name, type, repo, branch, primary_dash, owner_dash, cluster, concurrency, always_run, merge_blocking, qosGuaranteed, req.cpu (m), lim.cpu (m), req.mem (Gi), lim.mem (Gi)\n")
	for _, row := range rows {
		cols := []string{
			row.Date,
			row.Name,
			row.ProwJobType,
			row.Repo,
			row.Branch,
			row.PrimaryDashboard,
			row.OwnerDashboard,
			row.Cluster,
			strconv.Itoa(row.MaxConcurrency),
			strconv.FormatBool(row.AlwaysRun),
			strconv.FormatBool(row.MergeBlocking),
			strconv.FormatBool(row.QOSGuaranteed),
			strconv.FormatInt(row.RequestMilliCPU, 10),
			strconv.FormatInt(row.LimitMilliCPU, 10),
			strconv.FormatInt(row.RequestGigaMem, 10),
			strconv.FormatInt(row.LimitGigaMem, 10),
		}
		fmt.Println(strings.Join(cols, ", "))
	}
}

func PrintJSONReport(rows []ProwJobReportRow) {
	b, err := json.Marshal(rows)
	if err != nil {
		fmt.Println("error:", err)
	}
	os.Stdout.Write(b)
}

func PrintHTMLReport(rows []ProwJobReportRow) {
	t := template.Must(template.ParseGlob("./tpl/*"))
	err := t.ExecuteTemplate(os.Stdout, "report", rows)
	if err != nil {
		fmt.Print("execute: ", err)
		return
	}
}
