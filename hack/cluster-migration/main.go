/*
Copyright 2023 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
	cfg "k8s.io/test-infra/prow/config"
	"k8s.io/utils/strings/slices"
)

type Config struct {
	configPath    string
	jobConfigPath string
	repoReport    bool
}

type repoConfig struct {
	totalJobs     int
	completedJobs int
	eligibleJobs  int
	jobs          []jobConfig
}

type jobConfig struct {
	jobBase  cfg.JobBase
	eligible bool
}

type ClusterStat struct {
	EligibleCount int
	TotalCount    int
}

func (c *Config) validate() error {
	if c.configPath == "" {
		return fmt.Errorf("--config must set")
	}
	return nil
}

func loadConfig(configPath, jobConfigPath string) (*cfg.Config, error) {
	return cfg.Load(configPath, jobConfigPath, nil, "")
}

func reportTotalJobs(jobs map[string]repoConfig) {
	total := 0
	eligible := 0
	for _, job := range jobs {
		total += job.totalJobs
		eligible += job.eligibleJobs
	}
	fmt.Printf("Total jobs: %v\n", total)
	fmt.Printf("Total eligible jobs: %v\n", eligible)
	fmt.Printf("Currently ineligible jobs: %v\n", total-eligible)
}

func getRepo(path string) string {
	return strings.Split(path, "/")[1]
}

func main() {
	var config Config
	flag.StringVar(&config.configPath, "config", "../../config/prow/config.yaml", "Path to prow config")
	flag.StringVar(&config.jobConfigPath, "job-config", "../../config/jobs", "Path to prow job config")
	// flag.StringVar(&config.jobConfigPath, "job-config", "../../config/jobs", "Path to prow job config")
	flag.BoolVar(&config.repoReport, "repo-report", false, "Detailed report of all repo status")
	flag.Parse()

	if err := config.validate(); err != nil {
		log.Fatal(err)
	}

	c, err := loadConfig(config.configPath, config.jobConfigPath)
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
	}

	jobs := allStaticJobs(c)
	clusterStats := getClusterStatistics(jobs)
	repoStats := getRepoStatistics(jobs)

	reportTotalJobs(repoStats)
	printClusterStatistics(clusterStats)
	if config.repoReport {
		printRepoStatistics(repoStats)
	}
}

// The function "allStaticJobs" returns a sorted list of all static jobs from a given configuration.
func allStaticJobs(c *cfg.Config) map[string][]cfg.JobBase {
	jobs := map[string][]cfg.JobBase{}
	for key, postJobs := range c.JobConfig.PresubmitsStatic {
		for _, job := range postJobs {
			jobs[getRepo(key)] = append(jobs[getRepo(key)], job.JobBase)
		}
	}
	for key, postJobs := range c.JobConfig.PostsubmitsStatic {
		for _, job := range postJobs {
			jobs[getRepo(key)] = append(jobs[getRepo(key)], job.JobBase)
		}
	}
	for _, periodicJobs := range c.JobConfig.Periodics {
		key := strings.TrimPrefix(periodicJobs.JobBase.SourcePath, "../../config/jobs/")
		jobs[getRepo(key)] = append(jobs[getRepo(key)], periodicJobs.JobBase)
	}

	return jobs
}

func getPercentage(int1, int2 int) string {
	if int2 == 0 {
		return "100.00%"
	}
	return fmt.Sprintf("%.2f%%", float64(int1)/float64(int2)*100)
}

func getSortedKeys[V any](m map[string]V) []string {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}

	// Sort the keys in alphabetical order
	sort.Strings(keys)
	return keys
}

// The function checks if a job is eligible based on its cluster, labels, containers, and volumes.
func checkIfEligible(job cfg.JobBase) bool {
	validClusters := []string{"test-infra-trusted", "k8s-infra-prow-build", "k8s-infra-prow-build-trusted", "eks-prow-build-cluster"}
	if slices.Contains(validClusters, job.Cluster) {
		return true
	}
	if containsDisallowedLabel(job.Labels) {
		return false
	}
	for _, container := range job.Spec.Containers {
		if containsDisallowedAttributes(container) {
			return false
		}
	}
	return !containsDisallowedVolume(job.Spec.Volumes)
}

// The function checks if any label in a given map contains the substring "cred".
func containsDisallowedLabel(labels map[string]string) bool {
	for key := range labels {
		if strings.Contains(key, "cred") {
			return true
		}
	}
	return false
}

// The function checks if a container contains any disallowed attributes such as environment variables,
// arguments, or commands.
func containsDisallowedAttributes(container v1.Container) bool {
	for _, env := range container.Env {
		if strings.Contains(env.Name, "cred") {
			return true
		}
		if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
			return true
		}
	}
	disallowedArgs := []string{"gcloud", "gcp"}
	for _, arg := range container.Args {
		if containsAny(arg, disallowedArgs) {
			return true
		}
	}
	for _, cmd := range container.Command {
		if containsAny(cmd, disallowedArgs) {
			return true
		}
	}
	return containsDisallowedVolumeMount(container.VolumeMounts)
}

// The function "containsAny" checks if a given string contains any of the words in a given slice of
// strings.
func containsAny(s string, disallowed []string) bool {
	for _, word := range disallowed {
		if strings.Contains(strings.ToLower(s), word) {
			return true
		}
	}
	return false
}

// The function checks if any volume mount in a given list contains disallowed words in its name or
// mount path.
func containsDisallowedVolumeMount(volumeMounts []v1.VolumeMount) bool {
	disallowedWords := []string{"cred", "secret"}
	for _, vol := range volumeMounts {
		if containsAny(vol.Name, disallowedWords) || containsAny(vol.MountPath, disallowedWords) {
			return true
		}
	}
	return false
}

// The function checks if a list of volumes contains any disallowed volumes based on their name or if
// they are of type Secret.
func containsDisallowedVolume(volumes []v1.Volume) bool {
	for _, vol := range volumes {
		if strings.Contains(vol.Name, "cred") || vol.Secret != nil {
			return true
		}
	}
	return false
}

// The `getClusterStatistics` function takes a slice of `cfg.JobBase` objects as input and returns a
// map where the keys are cluster names and the values are slices of job names belonging to each
// cluster.
func getClusterStatistics(jobs map[string][]cfg.JobBase) map[string][]jobConfig {
	clusterMap := map[string][]jobConfig{}
	for _, job := range jobs {
		for _, j := range job {
			// Get the existing value from the map, or use the zero value if not present
			config := clusterMap[j.Cluster]
			jobConfig := jobConfig{
				jobBase:  j,
				eligible: checkIfEligible(j),
			}

			config = append(config, jobConfig)

			// Store the modified value back in the map
			clusterMap[j.Cluster] = config
		}
	}
	return clusterMap
}

// The `getClusterStatistics` function takes a slice of `cfg.JobBase` objects as input and returns a
// map where the keys are cluster names and the values are slices of job names belonging to each
// cluster.
func getRepoStatistics(jobs map[string][]cfg.JobBase) map[string]repoConfig {
	repoMap := map[string]repoConfig{}
	for key, job := range jobs {

		for _, j := range job {
			// fJob := strings.TrimPrefix(j.PathAlias, "k8s.io/")
			// fJob = strings.TrimPrefix(fJob, "sigs.k8s.io/")

			// Get the existing value from the map, or use the zero value if not present
			config := repoMap[key]
			jobConfig := jobConfig{
				jobBase:  j,
				eligible: checkIfEligible(j),
			}

			config.totalJobs++
			if j.Cluster != "default" {
				config.completedJobs++
			}

			if jobConfig.eligible {
				config.eligibleJobs++
			}

			config.jobs = append(config.jobs, jobConfig)

			// Store the modified value back in the map
			repoMap[key] = config
		}
	}
	return repoMap
}

func printClusterStatistics(clusterMap map[string][]jobConfig) {
	stats := aggregateClusterStatistics(clusterMap)
	printHeader()
	for _, key := range getSortedKeys(clusterMap) {
		printClusterStat(key, stats[key], stats)
	}
	printAggregateStats(stats)
}

func aggregateClusterStatistics(clusterMap map[string][]jobConfig) map[string]ClusterStat {
	stats := make(map[string]ClusterStat)
	for key, jobs := range clusterMap {
		total, eligible := 0, 0
		for _, j := range jobs {
			total++
			if j.eligible {
				eligible++
			}
		}
		stats[key] = ClusterStat{EligibleCount: eligible, TotalCount: total}
	}
	return stats
}

func printHeader() {
	format := "%-30v %-15v (%v)\n"
	header := fmt.Sprintf("\n"+format, "Cluster", "Eligible/Total", "Percent")
	separator := strings.Repeat("-", len(header))
	fmt.Print(header, separator+"\n")
}

func printClusterStat(clusterName string, stat ClusterStat, allStats map[string]ClusterStat) {
	format := "%-30v %-15v (%v/%v)\n"
	eligibleP := getPercentage(stat.EligibleCount, getTotalEligible(allStats))
	totalP := getPercentage(stat.TotalCount, getTotalJobs(allStats))
	fmt.Printf(format, clusterName, fmt.Sprintf("%v/%v", stat.EligibleCount, stat.TotalCount), eligibleP, totalP)
}

func printAggregateStats(allStats map[string]ClusterStat) {
	// Assuming "default" is GKE, aggregate other clusters as Community
	gkeStat, communityStat := allStats["default"], ClusterStat{}
	for key, stat := range allStats {
		if key != "default" {
			communityStat.EligibleCount += stat.EligibleCount
			communityStat.TotalCount += stat.TotalCount
		}
	}
	fmt.Printf("\nGoogle jobs    %v/%v (%v/%v)\n", gkeStat.EligibleCount, gkeStat.TotalCount, getPercentage(gkeStat.EligibleCount, getTotalEligible(allStats)), getPercentage(gkeStat.TotalCount, getTotalJobs(allStats)))
	fmt.Printf("Community jobs %v/%v (%v/%v)\n", communityStat.EligibleCount, communityStat.TotalCount, getPercentage(communityStat.EligibleCount, getTotalEligible(allStats)), getPercentage(communityStat.TotalCount, getTotalJobs(allStats)))
}

func getTotalEligible(allStats map[string]ClusterStat) int {
	total := 0
	for _, stat := range allStats {
		total += stat.EligibleCount
	}
	return total
}

func getTotalJobs(allStats map[string]ClusterStat) int {
	total := 0
	for _, stat := range allStats {
		total += stat.TotalCount
	}
	return total
}

// The function `printRepoStatistics` prints a report of repository statistics, including the number of
// jobs in each repository and the percentage of jobs in each repository compared to the total number of
// jobs.
func printRepoStatistics(repoMap map[string]repoConfig) {
	format := "%-55v  %-10v %-20v %-10v (%v)\n"
	header := fmt.Sprintf("\n"+format, "Repository", "Complete", "Eligible(Total)", "Remaining", "Percent")
	separator := strings.Repeat("-", len(header))

	fmt.Print(header)
	fmt.Println(separator)

	for _, key := range getSortedKeys(repoMap) {
		total := fmt.Sprintf("%v(%v)", repoMap[key].eligibleJobs, repoMap[key].totalJobs)
		remaining := repoMap[key].eligibleJobs - repoMap[key].completedJobs
		percent := getPercentage(repoMap[key].completedJobs, repoMap[key].eligibleJobs)
		fmt.Printf(format, key, repoMap[key].completedJobs, total, remaining, percent)
	}
}
