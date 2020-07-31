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
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"encoding/json"

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
var reportFormat = flag.String("format", "csv", "Output format [csv|json] defaults to csv")

// struct to report on stated resource requests and limits
// a empty strings field indicates that no request was found 
type RequestedResources struct {
	cpuRequested string ; // "req.cpu": String(r.Requests[corev1.ResourceCPU], resource.Milli),
	cpuLimitedTo string ; // "lim.cpu": String(r.Limits[corev1.ResourceCPU], resource.Milli),
	memRequested string ; // "req.mem": String(r.Requests[corev1.ResourceMemory], resource.Giga),
	memLimitedTo string ; // "lim.mem": String(r.Limits[corev1.ResourceCPU], resource.Giga),
}

// From a CI runtime perspective a Job has configuration data located in multiple locations
// using multiple means of specification.
// For the purposes of this report, the decoratedJobConfig contains JobBase config data
// along with associated data that we are currently interested in reporting on.
type DecoratedJobConfig struct {
	Job cfg.JobBase;
	Dashboard string;
	Cluster string;
	RequestedContainerResources map[string]map[string]string// Maps cntr name to RequestedResources 
}

// Loaded at TestMain.
var c *cfg.Config

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
	c = conf
	switch *reportFormat {
	case "csv":
		PrintCsvOfJobResources()
	case "json":
		PrintJSONJobResources()
	}
}

func allStaticJobs() []cfg.JobBase {
	jobs := []cfg.JobBase{}
	for _, job := range c.AllStaticPresubmits(nil) {
		jobs = append(jobs, job.JobBase)
	}
	for _, job := range c.AllStaticPostsubmits(nil) {
		jobs = append(jobs, job.JobBase)
	}
	for _, job := range c.AllPeriodics() {
		jobs = append(jobs, job.JobBase)
	}
	return jobs
}

func isPodQOSGuaranteed(spec *corev1.PodSpec) bool {
	isGuaranteed := true
	c := spec.Containers[0]
	zero := resource.MustParse("0")
	resources := []corev1.ResourceName{
		corev1.ResourceCPU,
		corev1.ResourceMemory,
	}
	for _, r := range resources {
		limit, ok := c.Resources.Limits[r]
		if !ok || limit.Cmp(zero) == 0 || limit.Cmp(c.Resources.Requests[r]) != 0 {
			isGuaranteed = false
		}
	}
	return isGuaranteed
}
// GetResources returns a map of container names to resources requested by that container
func GetResources(spec *corev1.PodSpec) map[string]map[string]string {
	m := make(map[string]map[string]string)
	if spec == nil {
		return m
	}
	// Range over all Containers present in spec.
	// Q : I have not managed to find any spces with more than one Container

	fmt.Printf("Container count is %d\n", len(spec.Containers));
  	for i,c := range spec.Containers {
		cntrName := c.Name + "-" + strconv.Itoa(i);
		r := c.Resources ;

		m = map[string]map[string]string{
			cntrName : {
			"req.cpu": String(r.Requests[corev1.ResourceCPU], resource.Milli),
			"lim.cpu": String(r.Limits[corev1.ResourceCPU], resource.Milli),
			"req.mem": String(r.Requests[corev1.ResourceMemory], resource.Giga),
				"lim.mem": String(r.Limits[corev1.ResourceCPU], resource.Giga),
			},
		}
	}
	return m
}

func String(q resource.Quantity, s resource.Scale) string {
	return strconv.FormatInt(q.ScaledValue(s), 10)
}

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

func PrintJSONJobResources() {

	var decoratedJobs []DecoratedJobConfig;
	for _, job := range c.JobConfig.AllStaticPresubmits(nil) {
		var cluster string = job.Cluster;
		var dashboard = job.Annotations["testgrid-dashboards"];
		var requestedResources = GetResources(job.Spec);
		decoratedJobs = append(decoratedJobs, DecoratedJobConfig{
			Cluster: cluster,
			Dashboard : dashboard,
			Job : job.JobBase,
			RequestedContainerResources : requestedResources,
		});
	}
	b, err := json.Marshal(decoratedJobs);
	if err != nil {
		fmt.Println("error:", err)
	}
	os.Stdout.Write(b)
}
func PrintCsvOfJobResources() {
	/** 
	fmt.Printf("primary dashboard, owner dashboard, name, type, repo, cluster, concurrency, req.cpu (m), lim.cpu (m), req.mem (Gi), lim.mem (Gi)\n")
	printJobBaseColumns := func(jobType, repo string, job cfg.JobBase) {
		name := job.Name
		primaryDash := PrimaryDashboard(job)
		ownerDash := OwnerDashboard(job)
		cluster := job.Cluster
		res := GetResources(job.Spec)
		cols := []string{primaryDash, ownerDash, name, jobType, repo, cluster, strconv.Itoa(job.MaxConcurrency), res["req.cpu"], res["lim.cpu"], res["req.mem"], res["lim.mem"]}
		fmt.Println(strings.Join(cols, ", "))
	}
	for _, job := range c.AllPeriodics() {
		// TODO: depending on whether decoration or bootstrap is used repo could be any number of repos
		printJobBaseColumns("periodic", "TODO", job.JobBase)
	}
	for repo, jobs := range c.PostsubmitsStatic {
		for _, job := range jobs {
			printJobBaseColumns("postsubmit", repo, job.JobBase)
		}
	}
	for repo, jobs := range c.PresubmitsStatic {
		for _, job := range jobs {
			printJobBaseColumns("presubmit", repo, job.JobBase)
		}
	}
*/
}

func TestQueryReleaseBlockingJobs() {
	jobs := allStaticJobs()

	filtered := []cfg.JobBase{}
	dashboardRE := regexp.MustCompile(`sig-release-master-blocking`)
	//dashboardRE := regexp.MustCompile(`sig-release-(1.[0-9]{2}|master)-blocking`)
	for _, job := range jobs {
		if dashboards, ok := job.Annotations["testgrid-dashboards"]; ok {
			if dashboardRE.MatchString(dashboards) {
				filtered = append(filtered, job)
			}
		}
	}
	jobs = filtered

	jobNamesByCluster := make(map[string][]string)
	for _, job := range jobs {
		if job.Cluster != "" {
			if jobNames, ok := jobNamesByCluster[job.Cluster]; !ok {
				jobNamesByCluster[job.Cluster] = []string{job.Name}
			} else {
				jobNamesByCluster[job.Cluster] = append(jobNames, job.Name)
			}
		}
	}
	for cluster, jobNames := range jobNamesByCluster {
		fmt.Printf("There are %4d of %4d sig-release-.*-blocking jobs that use cluster: %s\n", len(jobNames), len(jobs), cluster)
		for _, name := range jobNames {
			fmt.Printf("  - %s\n", name)
		}
	}
}

