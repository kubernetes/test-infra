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

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

var pullCommitRe = regexp.MustCompile(`^[-\w]+:\w{40},\d+:(\w{40})$`)

type prHistoryTemplate struct {
	Link    string
	Name    string
	Jobs    []prJobData
	Commits []commitData
}

type prJobData struct {
	Name   string
	Link   string
	Builds []buildData
}

type jobBuilds struct {
	name          string
	buildPrefixes []string
}

type commitData struct {
	Hash       string
	HashPrefix string // used only for display purposes, so don't worry about uniqueness
	Link       string
	MaxWidth   int
	latest     time.Time // time stamp of the job most recently started
}

type latestCommit []commitData

func (a latestCommit) Len() int      { return len(a) }
func (a latestCommit) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a latestCommit) Less(i, j int) bool {
	if len(a[i].Hash) != 40 {
		return true
	}
	if len(a[j].Hash) != 40 {
		return false
	}
	return a[i].latest.Before(a[j].latest)
}

type byStarted []buildData

func (a byStarted) Len() int           { return len(a) }
func (a byStarted) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byStarted) Less(i, j int) bool { return a[i].Started.Before(a[j].Started) }

func githubPRLink(org, repo string, pr int) string {
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", org, repo, pr)
}

func githubCommitLink(org, repo, commitHash string) string {
	return fmt.Sprintf("https://github.com/%s/%s/commit/%s", org, repo, commitHash)
}

func jobHistLink(bucketName, jobName string) string {
	return fmt.Sprintf("/job-history/%s/pr-logs/directory/%s", bucketName, jobName)
}

// gets the pull commit hash from metadata
func getPullCommitHash(pull string) (string, error) {
	match := pullCommitRe.FindStringSubmatch(pull)
	if len(match) != 2 {
		expected := "branch:hash,pullNumber:hash"
		return "", fmt.Errorf("unable to parse pull %q (expected %q)", pull, expected)
	}
	return match[1], nil
}

// listJobBuilds concurrently lists builds for the given job prefixes that have been run on a PR
func listJobBuilds(bucket storageBucket, jobPrefixes []string) []jobBuilds {
	jobch := make(chan jobBuilds)
	defer close(jobch)
	for i, jobPrefix := range jobPrefixes {
		go func(i int, jobPrefix string) {
			buildPrefixes, err := bucket.listSubDirs(jobPrefix)
			if err != nil {
				logrus.WithError(err).Warningf("Error getting builds for job %s", jobPrefix)
			}
			jobch <- jobBuilds{
				name:          path.Base(jobPrefix),
				buildPrefixes: buildPrefixes,
			}
		}(i, jobPrefix)
	}
	jobs := []jobBuilds{}
	for range jobPrefixes {
		job := <-jobch
		jobs = append(jobs, job)
	}
	return jobs
}

// getPRBuildData concurrently fetches metadata on each build of each job run on a PR
func getPRBuildData(bucket storageBucket, jobs []jobBuilds) []buildData {
	buildch := make(chan buildData)
	defer close(buildch)
	expected := 0
	for _, job := range jobs {
		for j, buildPrefix := range job.buildPrefixes {
			go func(j int, jobName, buildPrefix string) {
				build, err := getBuildData(bucket, buildPrefix)
				if err != nil {
					logrus.WithError(err).Warningf("build %s information incomplete", buildPrefix)
				}
				split := strings.Split(strings.TrimSuffix(buildPrefix, "/"), "/")
				build.SpyglassLink = path.Join(spyglassPrefix, bucket.getName(), buildPrefix)
				build.ID = split[len(split)-1]
				build.jobName = jobName
				build.prefix = buildPrefix
				build.index = j
				buildch <- build
			}(j, job.name, buildPrefix)
			expected++
		}
	}
	builds := []buildData{}
	for k := 0; k < expected; k++ {
		build := <-buildch
		builds = append(builds, build)
	}
	return builds
}

func updateCommitData(commits map[string]*commitData, org, repo, hash string, buildTime time.Time, width int) {
	commit, ok := commits[hash]
	if !ok {
		commits[hash] = &commitData{
			Hash:       hash,
			HashPrefix: hash,
		}
		commit = commits[hash]
		if len(hash) == 40 {
			commit.HashPrefix = hash[:7]
			commit.Link = githubCommitLink(org, repo, hash)
		}
	}
	if buildTime.After(commit.latest) {
		commit.latest = buildTime
	}
	if width > commit.MaxWidth {
		commit.MaxWidth = width
	}
}

func parsePullKey(key string) (org, repo string, pr int, err error) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		err = fmt.Errorf("malformed PR key: %s", key)
		return
	}
	pr, err = strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	return parts[0], parts[1], pr, nil
}

// getGCSDirsForPR returns a map from bucket names -> set of "directories" containing presubmit data
func getGCSDirsForPR(config *config.Config, org, repo string, pr int) (map[string]sets.String, error) {
	toSearch := make(map[string]sets.String)
	fullRepo := org + "/" + repo
	presubmits, ok := config.Presubmits[fullRepo]
	if !ok {
		return toSearch, fmt.Errorf("couldn't find presubmits for %q in config", fullRepo)
	}

	for _, presubmit := range presubmits {
		var gcsConfig *v1.GCSConfiguration
		if presubmit.DecorationConfig != nil && presubmit.DecorationConfig.GCSConfiguration != nil {
			gcsConfig = presubmit.DecorationConfig.GCSConfiguration
		} else {
			// for undecorated jobs assume the default
			gcsConfig = config.Plank.DefaultDecorationConfig.GCSConfiguration
		}

		gcsPath, _, _ := gcsupload.PathsForJob(gcsConfig, &downwardapi.JobSpec{
			Type: v1.PresubmitJob,
			Job:  presubmit.Name,
			Refs: &v1.Refs{
				Repo: repo,
				Org:  org,
				Pulls: []v1.Pull{
					{Number: pr},
				},
			},
		}, "")
		gcsPath, _ = path.Split(path.Clean(gcsPath))
		if _, ok := toSearch[gcsConfig.Bucket]; !ok {
			toSearch[gcsConfig.Bucket] = sets.String{}
		}
		toSearch[gcsConfig.Bucket].Insert(gcsPath)
	}
	return toSearch, nil
}

func getPRHistory(url *url.URL, config *config.Config, gcsClient *storage.Client) (prHistoryTemplate, error) {
	start := time.Now()
	template := prHistoryTemplate{}

	key := strings.TrimPrefix(url.Path, "/pr-history/")
	org, repo, pr, err := parsePullKey(key)
	if err != nil {
		return template, fmt.Errorf("failed to parse URL: %v", err)
	}
	template.Name = fmt.Sprintf("%s/%s #%d", org, repo, pr)
	template.Link = githubPRLink(org, repo, pr)

	toSearch, err := getGCSDirsForPR(config, org, repo, pr)
	if err != nil {
		return template, fmt.Errorf("failed to list GCS directories for PR %s: %v", template.Name, err)
	}

	builds := []buildData{}
	// job name -> commit hash -> list of builds
	jobCommitBuilds := make(map[string]map[string][]buildData)

	for bucketName, gcsPaths := range toSearch {
		bucket := gcsBucket{bucketName, gcsClient.Bucket(bucketName)}
		for gcsPath := range gcsPaths {
			jobPrefixes, err := bucket.listSubDirs(gcsPath)
			if err != nil {
				return template, fmt.Errorf("failed to get job names: %v", err)
			}
			// We assume job names to be unique, as enforced during config validation.
			for _, jobPrefix := range jobPrefixes {
				jobName := path.Base(jobPrefix)
				jobData := prJobData{
					Name: jobName,
					Link: jobHistLink(bucketName, jobName),
				}
				template.Jobs = append(template.Jobs, jobData)
				jobCommitBuilds[jobName] = make(map[string][]buildData)
			}
			jobs := listJobBuilds(bucket, jobPrefixes)
			builds = append(builds, getPRBuildData(bucket, jobs)...)
		}
	}

	commits := make(map[string]*commitData)
	for _, build := range builds {
		jobName := build.jobName
		hash := build.commitHash
		jobCommitBuilds[jobName][hash] = append(jobCommitBuilds[jobName][hash], build)
		updateCommitData(commits, org, repo, hash, build.Started, len(jobCommitBuilds[jobName][hash]))
	}
	for _, commit := range commits {
		template.Commits = append(template.Commits, *commit)
	}
	// builds are grouped by commit, then sorted by build start time (newest-first)
	sort.Sort(sort.Reverse(latestCommit(template.Commits)))
	for i, job := range template.Jobs {
		for _, commit := range template.Commits {
			builds := jobCommitBuilds[job.Name][commit.Hash]
			sort.Sort(sort.Reverse(byStarted(builds)))
			template.Jobs[i].Builds = append(template.Jobs[i].Builds, builds...)
			// pad empty spaces
			for k := len(builds); k < commit.MaxWidth; k++ {
				template.Jobs[i].Builds = append(template.Jobs[i].Builds, buildData{})
			}
		}
	}

	elapsed := time.Now().Sub(start)
	logrus.Infof("loaded %s in %v", url.Path, elapsed)

	return template, nil
}
