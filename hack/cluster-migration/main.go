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
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"sort"
	"strings"

	v1 "k8s.io/api/core/v1"
	cfg "sigs.k8s.io/prow/pkg/config"
)

type Config struct {
	configPath       string
	jobConfigPath    string
	repoReport       bool
	repo             string
	output           string
	ineligibleReport bool
	eligibleReport   bool
	todoReport       bool
}

type status struct {
	TotalJobs     int             `json:"totalJobs"`
	CompletedJobs int             `json:"completedJobs"`
	EligibleJobs  int             `json:"eligibleJobs"`
	Clusters      []clusterStatus `json:"clusters"`
}

type clusterStatus struct {
	ClusterName  string       `json:"clusterName"`
	EligibleJobs int          `json:"eligibleJobs"`
	TotalJobs    int          `json:"totalJobs"`
	RepoStatus   []repoStatus `json:"repoStatus"`
}

type repoStatus struct {
	RepoName     string      `json:"repoName"`
	EligibleJobs int         `json:"eligibleJobs"`
	TotalJobs    int         `json:"totalJobs"`
	Jobs         []jobStatus `json:"jobs"`
}

type jobStatus struct {
	JobName    string      `json:"jobName"`
	JobDetails cfg.JobBase `json:"jobDetails"`
	Eligible   bool        `json:"eligible"`
	Reason     string      `json:"reason"`
	SourcePath string      `json:"sourcePath"`
}

var communityClusters = map[string]bool{
	"k8s-infra-prow-build":         true,
	"k8s-infra-prow-build-trusted": true,
	"eks-prow-build-cluster":       true,
	"k8s-infra-kops-prow-build":    true,
}

func (j *jobStatus) IsMigrated() bool {
	return communityClusters[j.JobDetails.Cluster]
}

var config Config

var allowedSecretNames = []string{
	"service-account",
	"aws-credentials-607362164682",
	"aws-credentials-768319786644",
	"aws-credentials-boskos-scale-001-kops",
	"aws-ssh-key-secret",
	"ssh-key-secret",
}

var allowedLabelNames = []string{
	"preset-aws-credential",
	"preset-aws-ssh",
}

var allowedVolumeNames = []string{
	"aws-cred",
	"ssh",
}

var allowedEnvironmentVariables = []string{
	"GOOGLE_APPLICATION_CREDENTIALS_DEPRECATED",
	"E2E_GOOGLE_APPLICATION_CREDENTIALS",
	"GOOGLE_APPLICATION_CREDENTIALS",
	"AWS_SHARED_CREDENTIALS_FILE",
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

// The function "reportTotalJobs" prints the total number of jobs, completed jobs, and eligible jobs.
func reportTotalJobs(s status) {
	fmt.Printf("Total jobs: %v\n", s.TotalJobs)
	fmt.Printf("Completed jobs: %v\n", s.CompletedJobs)
	fmt.Printf("Eligible jobs: %v\n", s.EligibleJobs-s.CompletedJobs)
}

// The function "reportClusterStats" prints the statistics of each cluster in a sorted order.
func reportClusterStats(s status) {
	printHeader()
	sortedClusters := []string{}
	for _, cluster := range s.Clusters {
		sortedClusters = append(sortedClusters, cluster.ClusterName)
	}
	sort.Strings(sortedClusters)

	for _, cluster := range sortedClusters {
		for _, c := range s.Clusters {
			if c.ClusterName == cluster {
				if cluster == "default" {
					printDefaultClusterStats(cluster, c, s.Clusters)
					continue
				} else {
					printClusterStat(cluster, c, s.Clusters)
				}
			}
		}
	}
}

// The function "printHeader" prints a formatted header for displaying cluster information.
func printHeader() {
	format := "%-30v %-20v %v\n"
	header := fmt.Sprintf("\n"+format, "Cluster", "Total(Eligible)", "% of Total(% of Eligible)")
	separator := strings.Repeat("-", len(header))
	fmt.Print(header, separator+"\n")
}

func printDefaultClusterStats(clusterName string, stat clusterStatus, allStats []clusterStatus) {
	format := "%-30v %-20v %-10v(%v)\n"
	eligibleP := getPercentage(stat.EligibleJobs, getTotalEligible(allStats))
	totalP := getPercentage(stat.TotalJobs, getTotalJobs(allStats))
	fmt.Printf(format, clusterName, fmt.Sprintf("%v(%v)", stat.TotalJobs, stat.EligibleJobs), printPercentage(totalP), printPercentage(eligibleP))
}

// The function "printClusterStat" prints the status of a cluster, including the number of eligible and
// total jobs, as well as the percentage of eligible and total jobs compared to all clusters.
func printClusterStat(clusterName string, stat clusterStatus, allStats []clusterStatus) {
	format := "%-30v %-20v %-10v(%v)\n"
	eligibleP := getPercentage(stat.EligibleJobs, getTotalEligible(allStats))
	totalP := getPercentage(stat.TotalJobs, getTotalJobs(allStats))
	fmt.Printf(format, clusterName, stat.TotalJobs, printPercentage(totalP), printPercentage(eligibleP))
}

// The function `getTotalEligible` calculates the total number of eligible jobs from a given list of
// cluster statuses.
func getTotalEligible(allStats []clusterStatus) int {
	total := 0
	for _, stat := range allStats {
		total += stat.EligibleJobs
	}
	return total
}

// The function "getTotalJobs" calculates the total number of jobs from a given slice of clusterStatus
// structs.
func getTotalJobs(allStats []clusterStatus) int {
	total := 0
	for _, stat := range allStats {
		total += stat.TotalJobs
	}
	return total
}

func getAllRepos(s status) []string {
	repos := []string{}
	for _, cluster := range s.Clusters {
		for _, repo := range cluster.RepoStatus {
			if !slices.Contains(repos, repo.RepoName) {
				repos = append(repos, repo.RepoName)
			}
		}
	}
	return repos
}

// The function `printRepoStatistics` prints statistics for repositories, including completion status,
// eligibility, remaining jobs, and percentage complete.
func printRepoStatistics(s status) {
	format := "%-55v  %-10v %-20v %-10v (%v)\n"
	header := fmt.Sprintf("\n"+format, "Repository", "Complete", "Total(Eligible)", "Remaining", "Percent")
	separator := strings.Repeat("-", len(header))

	fmt.Print(header)
	fmt.Println(separator)

	sortedRepos := []string{}
	for _, cluster := range s.Clusters {
		for _, repo := range cluster.RepoStatus {
			if !slices.Contains(sortedRepos, repo.RepoName) {
				sortedRepos = append(sortedRepos, repo.RepoName)
			}
		}
	}
	sort.Strings(sortedRepos)

	for _, repo := range sortedRepos {
		total := 0
		complete := 0
		eligible := 0
		for _, cluster := range s.Clusters {
			for _, r := range cluster.RepoStatus {
				if r.RepoName == repo {
					total += r.TotalJobs
					eligible += r.EligibleJobs
					if cluster.ClusterName != "default" {
						complete += r.TotalJobs
					}
				}
			}
		}
		remaining := eligible - complete
		percent := getPercentage(complete, eligible)
		fmt.Printf(format, repo, complete, fmt.Sprintf("%v(%v)", total, eligible), remaining, printPercentage(percent))
	}
}

func getRepo(path string) string {
	return strings.Split(path, "/")[1]
}

// The function `getStatus` calculates the status of jobs based on their clusters and repositories.
func getStatus(jobs map[string][]cfg.JobBase) status {
	s := status{}
	for repo, jobConfigs := range jobs {
		for _, job := range jobConfigs {
			cluster, eligible, ineligibleReason := getJobStatus(job)
			s.TotalJobs++
			if cluster != "" && cluster != "default" {
				s.CompletedJobs++
			} else {
				cluster = "default"
			}
			if eligible {
				s.EligibleJobs++
			}
			if !containsCluster(s.Clusters, cluster) {
				s.Clusters = append(s.Clusters, clusterStatus{ClusterName: cluster})
			}
			for i, c := range s.Clusters {
				if c.ClusterName == cluster {
					s.Clusters[i].TotalJobs++
					if eligible {
						s.Clusters[i].EligibleJobs++
					}
					if !containsRepo(s.Clusters[i].RepoStatus, repo) {
						s.Clusters[i].RepoStatus = append(s.Clusters[i].RepoStatus, repoStatus{RepoName: repo})
					}
					for j, r := range s.Clusters[i].RepoStatus {
						if r.RepoName == repo {
							s.Clusters[i].RepoStatus[j].TotalJobs++
							if eligible {
								s.Clusters[i].RepoStatus[j].EligibleJobs++
							}
							s.Clusters[i].RepoStatus[j].Jobs = append(s.Clusters[i].RepoStatus[j].Jobs, jobStatus{JobName: job.Name, JobDetails: job, Eligible: eligible, Reason: ineligibleReason, SourcePath: job.SourcePath})
						}
					}
				}
			}
		}
	}
	return s
}

func getJobStatus(job cfg.JobBase) (string, bool, string) {
	if job.Cluster != "default" {
		return job.Cluster, true, ""
	}

	eligible, ineligibleReason := checkIfEligible(job)

	return "", eligible, ineligibleReason
}

func containsCluster(clusters []clusterStatus, cluster string) bool {
	for _, c := range clusters {
		if c.ClusterName == cluster {
			return true
		}
	}
	return false
}

func containsRepo(repos []repoStatus, repo string) bool {
	for _, r := range repos {
		if r.RepoName == repo {
			return true
		}
	}
	return false
}

func getIncompleteJobs(repo string, status status) []jobStatus {
	ret := []jobStatus{}
	for _, cluster := range status.Clusters {
		for _, repoStatus := range cluster.RepoStatus {
			if repoStatus.RepoName == repo {
				for _, job := range repoStatus.Jobs {
					if !job.IsMigrated() {
						ret = append(ret, job)
					}
				}
			}
		}
	}
	return ret
}

// The function `printJobStats` prints the status of jobs in a given repository,
func printJobStats(repo string, status status, onlyIneligible bool, onlyEligible bool) {
	format := "%-70v is %s%v\033[0m\n" // \033[0m resets color back to default after printing

	for _, cluster := range status.Clusters {
		for _, repoStatus := range cluster.RepoStatus {
			if repoStatus.RepoName == repo {
				for _, job := range repoStatus.Jobs {
					if onlyIneligible && job.Eligible {
						continue
					}
					if onlyEligible && !job.Eligible || cluster.ClusterName != "default" {
						continue
					}

					if cluster.ClusterName != "default" {
						fmt.Printf(format, job.JobName, "\033[33m", "done") // \033[33m sets text color to yellow
					} else if job.Eligible {
						fmt.Printf(format, job.JobName, "\033[32m", "eligible") // \033[32m sets text color to green
					} else {
						fmt.Printf(format, job.JobName, "\033[31m", "not eligible ("+job.Reason+")") // \033[31m sets text color to red
					}
				}
			}
		}
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

func getPercentage(int1, int2 int) float64 {
	if int2 == 0 {
		return 100
	}
	return float64(int1) / float64(int2) * 100
}

func printPercentage(f float64) string {
	return fmt.Sprintf("%.2f%%", f)
}

// checkIfEligible determines if a given job is eligible based on its cluster, labels, containers, and volumes.
// To be eligible:
// - The job must belong to one of the specified valid community clusters.
// - The job's labels must not contain any disallowed substrings. The only current disallowed substring is "cred".
// - The job's containers must not have any disallowed attributes. The disallowed attributes include:
//   - Environment variables containing the substring "cred".
//   - Environment variables derived from secrets.
//   - Arguments containing any of the disallowed arguments.
//   - Commands containing any of the disallowed commands.
//   - Volume mounts containing any of the disallowed words like "cred" or "secret".
//
// - The job's volumes must not contain any disallowed volumes. Volumes are considered disallowed if:
//   - Their name contains the substring "cred".
//   - They are of type Secret but their name is not in the list of allowed secret names.
func checkIfEligible(job cfg.JobBase) (bool, string) {
	validClusters := []string{"test-infra-trusted", "k8s-infra-prow-build", "k8s-infra-prow-build-trusted", "eks-prow-build-cluster"}
	if slices.Contains(validClusters, job.Cluster) {
		return true, ""
	}
	if ok, reason := containsDisallowedLabel(job.Labels); ok {
		return false, reason
	}

	for _, container := range job.Spec.Containers {
		if ok, reason := containsDisallowedAttributes(container); ok {
			return false, reason
		}
	}

	if ok, reason := containsDisallowedVolume(job.Spec.Volumes); ok {
		return false, reason
	}

	if job.Spec.ServiceAccountName != "" && job.Spec.ServiceAccountName != "prowjob-default-sa" {
		return false, "disallowed service account - " + job.Spec.ServiceAccountName
	}

	return true, ""
}

// The function checks if any label in a given map contains the substring "cred".
func containsDisallowedLabel(labels map[string]string) (bool, string) {
	for key := range labels {
		if checkContains(key, "cred") && !labelIsAllowed(key) {
			return true, "disallowed label - " + key
		}
	}
	return false, ""
}

func checkContains(s string, substring string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substring))
}

func labelIsAllowed(label string) bool {
	for _, allowedLabel := range allowedLabelNames {
		if checkContains(label, allowedLabel) {
			return true
		}
	}
	return false
}

func volumeIsAllowed(volumeName string) bool {
	for _, allowedVolume := range allowedVolumeNames {
		if volumeName == allowedVolume {
			return true
		}
	}
	return false
}

func secretIsAllowed(secretName string) bool {
	for _, allowedSecret := range allowedSecretNames {
		if secretName == allowedSecret {
			return true
		}
	}
	return false
}

func envVarIsAllowed(envVar string) bool {
	for _, allowedEnvVar := range allowedEnvironmentVariables {
		if allowedEnvVar == envVar {
			return true
		}
	}
	return false
}

// The function checks if a container contains any disallowed attributes such as environment variables,
// arguments, or commands.
func containsDisallowedAttributes(container v1.Container) (bool, string) {
	for _, env := range container.Env {
		if checkContains(env.Name, "cred") && !envVarIsAllowed(env.Name) {
			return true, "disallowed environment variable - " + env.Name
		}
		if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil && !secretIsAllowed(env.ValueFrom.SecretKeyRef.Key) {
			return true, "disallowed environment variable - " + env.Name
		}
	}
	if ok, reason := containsDisallowedVolumeMount(container.VolumeMounts); ok {
		return true, reason
	}

	return false, ""
}

// The function "containsAny" checks if a given string contains any of the words in a given slice of
// strings.
func containsAny(s string, disallowed []string) bool {
	for _, word := range disallowed {
		if checkContains(s, word) {
			return true
		}
	}
	return false
}

// The function checks if any volume mount in a given list contains disallowed words in its name or
// mount path.
func containsDisallowedVolumeMount(volumeMounts []v1.VolumeMount) (bool, string) {
	disallowedWords := []string{"cred", "secret"}
	for _, vol := range volumeMounts {
		if (containsAny(vol.Name, disallowedWords) || containsAny(vol.MountPath, disallowedWords)) && !volumeIsAllowed(vol.Name) {
			return true, "disallowed volume mount - " + vol.Name
		}
	}
	return false, ""
}

// The function checks if a list of volumes contains any disallowed volumes based on their name or if
// they are of type Secret.
func containsDisallowedVolume(volumes []v1.Volume) (bool, string) {
	for _, vol := range volumes {
		if (checkContains(vol.Name, "cred") && !volumeIsAllowed(vol.Name)) || (vol.Secret != nil && !secretIsAllowed(vol.Secret.SecretName)) {
			return true, "disallowed volume - " + vol.Name
		}
	}
	return false, ""
}

func volumeSecrets(volumes []v1.Volume) (s []string) {
	for _, vol := range volumes {
		if vol.Secret != nil {
			s = append(s, vol.Secret.SecretName)
		}
	}
	return s
}

func main() {
	flag.StringVar(&config.configPath, "config", "../../config/prow/config.yaml", "Path to prow config")
	flag.StringVar(&config.jobConfigPath, "job-config", "../../config/jobs", "Path to prow job config")
	flag.StringVar(&config.repo, "repo", "", "Find eligible jobs for a specific repo")
	flag.StringVar(&config.output, "output", "", "Output format (default, json)")
	flag.BoolVar(&config.repoReport, "repo-report", false, "Detailed report of all repo status")
	flag.BoolVar(&config.ineligibleReport, "ineligible-report", false, "Get a detailed report of ineligible jobs")
	flag.BoolVar(&config.eligibleReport, "eligible-report", false, "Get a detailed report of eligible jobs")
	flag.BoolVar(&config.todoReport, "todo-report", false, "Get a detailed report of jobs that are not yet completed")
	flag.Parse()

	if err := config.validate(); err != nil {
		log.Fatal(err)
	}

	c, err := loadConfig(config.configPath, config.jobConfigPath)
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
	}

	jobs := allStaticJobs(c)
	status := getStatus(jobs)

	if config.output == "json" {
		bt, err := json.Marshal(status)
		if err != nil {
			log.Fatal(err)
		}

		var out bytes.Buffer
		json.Indent(&out, bt, "", " ")
		out.WriteTo(os.Stdout)
		println("\n")
		return
	}

	if config.todoReport {
		// Create an output html file
		f, err := os.Create("job-migration-todo.md")
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		// Write the html header
		f.WriteString(`
## JOBS AT RISK

If you own any jobs listed below, PLEASE ensure they are migrated to a community cluster prior to August 1st, 2024. If you need help, please reach out to #sig-testing of #sig-k8s-infra on Slack.

For more context, see the announcement thread at https://groups.google.com/a/kubernetes.io/g/dev/c/p6PAML90ZOU

| Secrets | File Path | Job | Link |
| --- | --- | --- | --- |
`)
		var results []string
		repos := getAllRepos(status)
		for _, repo := range repos {
			jobs := getIncompleteJobs(repo, status)
			for _, job := range jobs {
				secrets := volumeSecrets(job.JobDetails.Spec.Volumes)
				link := "https://cs.k8s.io/?q=name%3A%20" + job.JobName + "%24&i=nope&files=&excludeFiles=&repos="
				results = append(results, fmt.Sprintf("%s\t%s\t%v\t%s", secrets, job.SourcePath, job.JobName, link))
			}
		}
		sort.Strings(results)
		for _, line := range results {
			parts := strings.Split(line, "\t")
			_, err = f.WriteString(fmt.Sprintf("|%v|%v|%v|[Search Results](%s)|\n", parts[0], parts[1], parts[2], parts[3]))
			if err != nil {
				log.Fatal(err)
			}
		}
		return
	}

	if config.ineligibleReport {
		for _, repo := range getAllRepos(status) {
			fmt.Println("\nRepo: " + repo)
			printJobStats(repo, status, true, false)
		}
		return
	}

	if config.eligibleReport {
		for _, repo := range getAllRepos(status) {
			fmt.Println("\nRepo: " + repo)
			printJobStats(repo, status, false, true)
		}
		return
	}

	if config.repo != "" {
		printJobStats(config.repo, status, false, false)
		return
	}

	reportTotalJobs(status)
	reportClusterStats(status)
	if config.repoReport {
		printRepoStatistics(status)
	}
}
