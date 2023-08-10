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

	cfg "k8s.io/test-infra/prow/config"
)

type Config struct {
	configPath    string
	jobConfigPath string
	repoReport    bool
}

type jobConfig struct {
	totalJobs     int
	completedJobs int
	jobs          []string
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

func reportTotalJobs(total int) {
	fmt.Printf("Total jobs: %v\n", total)
}

func getRepo(path string) string {
	return strings.Split(path, "/")[1]
}

func main() {
	var config Config
	flag.StringVar(&config.configPath, "config", "../../config/prow/config.yaml", "Path to prow config")
	flag.StringVar(&config.jobConfigPath, "job-config", "../../config/jobs", "Path to prow job config")
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
	printClusterStatistics(clusterStats, len(jobs))
	if config.repoReport {
		repoStats := getRepoStatistics(jobs)
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

// The `getClusterStatistics` function takes a slice of `cfg.JobBase` objects as input and returns a
// map where the keys are cluster names and the values are slices of job names belonging to each
// cluster.
func getClusterStatistics(jobs map[string][]cfg.JobBase) map[string][]string {
	clusterMap := map[string][]string{}
	for _, job := range jobs {
		for _, j := range job {
			clusterMap[j.Cluster] = append(clusterMap[j.Cluster], j.Name)
		}
	}
	return clusterMap
}

// The `getClusterStatistics` function takes a slice of `cfg.JobBase` objects as input and returns a
// map where the keys are cluster names and the values are slices of job names belonging to each
// cluster.
func getRepoStatistics(jobs map[string][]cfg.JobBase) map[string]jobConfig {
	repoMap := map[string]jobConfig{}
	for key, job := range jobs {

		for _, j := range job {
			// fJob := strings.TrimPrefix(j.PathAlias, "k8s.io/")
			// fJob = strings.TrimPrefix(fJob, "sigs.k8s.io/")

			// Get the existing value from the map, or use the zero value if not present
			config := repoMap[key]

			config.totalJobs++
			if j.Cluster != "default" {
				config.completedJobs++
			}

			config.jobs = append(config.jobs, j.Name)

			// Store the modified value back in the map
			repoMap[key] = config
		}
	}
	return repoMap
}

// The function `printClusterStatistics` prints a report of cluster statistics, including the number of
// jobs in each cluster and the percentage of jobs in each cluster compared to the total number of
// jobs.
func printClusterStatistics(clusterMap map[string][]string, total int) {
	reportTotalJobs(total)

	header := fmt.Sprintf("\n%-30v %-5v %v", "Cluster", "Jobs", "Percent")
	separator := strings.Repeat("-", len(header))

	fmt.Println(header)
	fmt.Println(separator)

	gkeJobs := 0
	communityJobs := 0

	for _, key := range getSortedKeys(clusterMap) {
		if key == "default" {
			gkeJobs += len(clusterMap[key])
		} else {
			communityJobs += len(clusterMap[key])
		}
		fmt.Printf("%-30v %-5v (%v)\n", key, len(clusterMap[key]), getPercentage(len(clusterMap[key]), total))
	}

	fmt.Printf("\nGoogle jobs    %v (%v)\n", gkeJobs, getPercentage(gkeJobs, total))
	fmt.Printf("Community jobs %v (%v)\n", communityJobs, getPercentage(communityJobs, total))
}

// The function `printRepoStatistics` prints a report of repository statistics, including the number of
// jobs in each repository and the percentage of jobs in each repository compared to the total number of
// jobs.
func printRepoStatistics(repoMap map[string]jobConfig) {
	header := fmt.Sprintf("\n%-50v %-10v %-10v %-10v %v", "Repository", "Complete", "Total", "Remaining", "Percent")
	separator := strings.Repeat("-", len(header))

	fmt.Println(header)
	fmt.Println(separator)

	for _, key := range getSortedKeys(repoMap) {
		if len(key) == 0 {
			for _, job := range repoMap[key].jobs {
				fmt.Printf("%-50v  %-10v %-10v %-10v (%v)\n", "  "+job, repoMap[key].completedJobs, repoMap[key].totalJobs, repoMap[key].totalJobs-repoMap[key].completedJobs, getPercentage(repoMap[key].completedJobs, repoMap[key].totalJobs))
			}
		} else {
			fmt.Printf("%-50v  %-10v %-10v %-10v (%v)\n", key, repoMap[key].completedJobs, repoMap[key].totalJobs, repoMap[key].totalJobs-repoMap[key].completedJobs, getPercentage(repoMap[key].completedJobs, repoMap[key].totalJobs))
		}
	}
}
