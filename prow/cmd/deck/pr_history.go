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
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

var pullCommitRe = regexp.MustCompile(`^[-\.\w]+:\w{40},\d+:(\w{40})$`)

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

func githubPRLink(githubHost, org, repo string, pr int) string {
	return fmt.Sprintf("https://%s/%s/%s/pull/%d", githubHost, org, repo, pr)
}

func githubCommitLink(githubHost, org, repo, commitHash string) string {
	return fmt.Sprintf("https://%s/%s/%s/commit/%s", githubHost, org, repo, commitHash)
}

func jobHistLink(storageProvider, bucketName, jobName string) string {
	return fmt.Sprintf("/job-history/%s/%s/pr-logs/directory/%s", storageProvider, bucketName, jobName)
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
func listJobBuilds(ctx context.Context, bucket storageBucket, jobPrefixes []string) []jobBuilds {
	jobch := make(chan jobBuilds)
	defer close(jobch)
	for i, jobPrefix := range jobPrefixes {
		go func(i int, jobPrefix string) {
			buildPrefixes, err := bucket.listSubDirs(ctx, jobPrefix)
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
func getPRBuildData(ctx context.Context, bucket storageBucket, jobs []jobBuilds) []buildData {
	buildch := make(chan buildData)
	defer close(buildch)
	expected := 0
	for _, job := range jobs {
		for j, buildPrefix := range job.buildPrefixes {
			go func(j int, jobName, buildPrefix string) {
				build, err := getBuildData(ctx, bucket, buildPrefix)
				if err != nil {
					logrus.WithError(err).Warningf("build %s information incomplete", buildPrefix)
				}
				split := strings.Split(strings.TrimSuffix(buildPrefix, "/"), "/")
				build.SpyglassLink = path.Join(spyglassPrefix, bucket.getStorageProvider(), bucket.getName(), buildPrefix)
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

func updateCommitData(commits map[string]*commitData, githubHost, org, repo, hash string, buildTime time.Time, width int) {
	commit, ok := commits[hash]
	if !ok {
		commits[hash] = &commitData{
			Hash:       hash,
			HashPrefix: hash,
		}
		commit = commits[hash]
		if len(hash) == 40 {
			commit.HashPrefix = hash[:7]
			commit.Link = githubCommitLink(githubHost, org, repo, hash)
		}
	}
	if buildTime.After(commit.latest) {
		commit.latest = buildTime
	}
	if width > commit.MaxWidth {
		commit.MaxWidth = width
	}
}

// parsePullURL parses PR history URLs. Expects this format:
// .../pr-history?org=<org>&repo=<repo>&pr=<pr number>
func parsePullURL(u *url.URL) (org, repo string, pr int, err error) {
	var prStr string
	vals := u.Query()
	if org = vals.Get("org"); org == "" {
		return "", "", 0, fmt.Errorf("no value provided for org")
	}
	if repo = vals.Get("repo"); repo == "" {
		return "", "", 0, fmt.Errorf("no value provided for repo")
	}
	prStr = vals.Get("pr")
	pr, err = strconv.Atoi(prStr)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid PR number %q: %v", prStr, err)
	}
	return org, repo, pr, nil
}

// getStorageDirsForPR returns a map from bucket names -> set of "directories" containing presubmit data
func getStorageDirsForPR(c *config.Config, gitHubClient deckGitHubClient, gitClient git.ClientFactory, org, repo string, prNumber int) (map[string]sets.String, error) {
	toSearch := make(map[string]sets.String)
	fullRepo := org + "/" + repo

	if c.InRepoConfigEnabled(fullRepo) && gitHubClient == nil {
		return nil, errors.New("inrepoconfig is enabled but no --github-token-path configured on deck")
	}
	prRefGetter := config.NewRefGetterForGitHubPullRequest(gitHubClient, org, repo, prNumber)
	presubmits, err := c.GetPresubmits(gitClient, org+"/"+repo, prRefGetter.BaseSHA, prRefGetter.HeadSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get Presubmits for pull request %s/%s#%d: %v", org, repo, prNumber, err)
	}
	if len(presubmits) == 0 {
		return toSearch, fmt.Errorf("couldn't find presubmits for %q in config", fullRepo)
	}

	for _, presubmit := range presubmits {
		var gcsConfig *v1.GCSConfiguration
		if presubmit.DecorationConfig != nil && presubmit.DecorationConfig.GCSConfiguration != nil {
			gcsConfig = presubmit.DecorationConfig.GCSConfiguration
		} else {
			// for undecorated jobs assume the default
			def := c.Plank.GuessDefaultDecorationConfig(fullRepo, presubmit.Cluster)
			if def == nil || def.GCSConfiguration == nil {
				return toSearch, fmt.Errorf("failed to guess gcs config based on default decoration config: %w", err)
			}
			gcsConfig = def.GCSConfiguration
		}

		gcsPath, _, _ := gcsupload.PathsForJob(gcsConfig, &downwardapi.JobSpec{
			Type: v1.PresubmitJob,
			Job:  presubmit.Name,
			Refs: &v1.Refs{
				Repo: repo,
				Org:  org,
				Pulls: []v1.Pull{
					{Number: prNumber},
				},
			},
		}, "")
		gcsPath, _ = path.Split(path.Clean(gcsPath))
		bucketName := gcsConfig.Bucket
		// bucket is the bucket field of the GCSConfiguration, which means it could be missing the
		// storageProvider prefix (but it's deprecated to use a bucket name without <storage-type>:// prefix)
		if !strings.Contains(bucketName, "://") {
			bucketName = "gs://" + bucketName
		}
		if _, ok := toSearch[bucketName]; !ok {
			toSearch[bucketName] = sets.String{}
		}
		toSearch[bucketName].Insert(gcsPath)
	}
	return toSearch, nil
}

func getPRHistory(ctx context.Context, prHistoryURL *url.URL, config *config.Config, opener io.Opener, gitHubClient deckGitHubClient, gitClient git.ClientFactory, githubHost string) (prHistoryTemplate, error) {
	start := time.Now()
	template := prHistoryTemplate{}

	org, repo, pr, err := parsePullURL(prHistoryURL)
	if err != nil {
		return template, fmt.Errorf("failed to parse URL %s: %v", prHistoryURL.String(), err)
	}
	template.Name = fmt.Sprintf("%s/%s #%d", org, repo, pr)
	template.Link = githubPRLink(githubHost, org, repo, pr) // TODO(ibzib) support Gerrit :/

	toSearch, err := getStorageDirsForPR(config, gitHubClient, gitClient, org, repo, pr)
	if err != nil {
		return template, fmt.Errorf("failed to list directories for PR %s: %v", template.Name, err)
	}

	builds := []buildData{}
	// job name -> commit hash -> list of builds
	jobCommitBuilds := make(map[string]map[string][]buildData)

	for bucket, storagePaths := range toSearch {
		parsedBucket, err := url.Parse(bucket)
		if err != nil {
			return template, fmt.Errorf("parse bucket %s: %w", bucket, err)
		}
		bucketName := parsedBucket.Host
		storageProvider := parsedBucket.Scheme
		bucket, err := newBlobStorageBucket(bucketName, storageProvider, config, opener)
		if err != nil {
			return template, err
		}
		for storagePath := range storagePaths {
			jobPrefixes, err := bucket.listSubDirs(ctx, storagePath)
			if err != nil {
				return template, fmt.Errorf("failed to get job names: %v", err)
			}
			// We assume job names to be unique, as enforced during config validation.
			for _, jobPrefix := range jobPrefixes {
				jobName := path.Base(jobPrefix)
				jobData := prJobData{
					Name: jobName,
					Link: jobHistLink(storageProvider, bucketName, jobName),
				}
				template.Jobs = append(template.Jobs, jobData)
				jobCommitBuilds[jobName] = make(map[string][]buildData)
			}
			jobs := listJobBuilds(ctx, bucket, jobPrefixes)
			builds = append(builds, getPRBuildData(ctx, bucket, jobs)...)
		}
	}

	commits := make(map[string]*commitData)
	for _, build := range builds {
		jobName := build.jobName
		hash := build.commitHash
		jobCommitBuilds[jobName][hash] = append(jobCommitBuilds[jobName][hash], build)
		updateCommitData(commits, githubHost, org, repo, hash, build.Started, len(jobCommitBuilds[jobName][hash]))
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

	elapsed := time.Since(start)
	logrus.WithField("duration", elapsed.String()).Infof("loaded %s", prHistoryURL.Path)

	return template, nil
}
