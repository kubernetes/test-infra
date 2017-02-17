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

// TODO(spxtr): Refactor and test this properly. It's getting out of hand.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/jenkins"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/line"
)

var (
	job       = flag.String("job-name", "", "Which Jenkins job to build.")
	repoOwner = flag.String("repo-owner", "", "Owner of the repo.")
	repoName  = flag.String("repo-name", "", "Name of the repo to test.")
	pr        = flag.Int("pr", 0, "Pull request to test.")
	author    = flag.String("author", "", "Author of the PR.")
	baseRef   = flag.String("base-ref", "", "Target branch.")
	baseSHA   = flag.String("base-sha", "", "Base SHA of the PR.")
	pullSHA   = flag.String("pull-sha", "", "Head SHA of the PR.")
	refs      = flag.String("refs", "", "Refs to merge together, as expected by bootstrap.py.")

	namespace = flag.String("namespace", "default", "Namespace that we live in.")
	dryRun    = flag.Bool("dry-run", true, "Whether or not to make mutating Jenkins calls.")
	report    = flag.Bool("report", true, "Whether or not to report the status on GitHub.")

	configPath       = flag.String("config-path", "/etc/config/config", "Where is config.yaml.")
	labelsPath       = flag.String("labels-path", "/etc/labels/labels", "Where our metadata.labels are mounted.")
	jenkinsURL       = flag.String("jenkins-url", "http://pull-jenkins-master:8080", "Jenkins URL")
	jenkinsUserName  = flag.String("jenkins-user", "jenkins-trigger", "Jenkins username")
	jenkinsTokenFile = flag.String("jenkins-token-file", "/etc/jenkins/jenkins", "Path to the file containing the Jenkins API token.")
	totURL           = flag.String("tot-url", "http://tot", "Tot URL")
	crierURL         = flag.String("crier-url", "http://crier", "Crier URL")
)

const (
	guberBasePR     = "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull"
	guberBasePush   = "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/logs"
	testInfra       = "https://github.com/kubernetes/test-infra/issues"
	agentKubernetes = "kubernetes"
	agentJenkins    = "jenkins"
)

type testClient struct {
	IsPresubmit bool
	Presubmit   *config.Presubmit
	Postsubmit  *config.Postsubmit

	JobName   string
	RepoOwner string
	RepoName  string
	PRNumber  int
	Author    string
	BaseRef   string
	BaseSHA   string
	PullSHA   string
	Refs      string

	DryRun bool
	Report bool

	KubeJob       string
	KubeClient    *kube.Client
	JenkinsClient *jenkins.Client
}

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	jenkinsSecretRaw, err := ioutil.ReadFile(*jenkinsTokenFile)
	if err != nil {
		logrus.WithError(err).Fatalf("Could not read token file.")
	}
	jenkinsToken := string(bytes.TrimSpace(jenkinsSecretRaw))

	var jenkinsClient *jenkins.Client
	if *dryRun {
		jenkinsClient = jenkins.NewDryRunClient(*jenkinsURL, *jenkinsUserName, jenkinsToken)
	} else {
		jenkinsClient = jenkins.NewClient(*jenkinsURL, *jenkinsUserName, jenkinsToken)
	}

	kc, err := kube.NewClientInCluster(*namespace)
	if err != nil {
		logrus.Fatalf("Error getting client: %v", err)
	}

	kubeJob, err := getKubeJob(*labelsPath)
	if err != nil {
		logrus.Fatalf("Error getting kube job name: %v", err)
	}

	c, err := config.Load(*configPath)
	if err != nil {
		logrus.WithError(err).Fatal("Error loading config.")
	}
	fullRepoName := fmt.Sprintf("%s/%s", *repoOwner, *repoName)
	foundPresubmit, presubmit := c.GetPresubmit(fullRepoName, *job)
	foundPostsubmit, postsubmit := c.GetPostsubmit(fullRepoName, *job)
	if !foundPresubmit && !foundPostsubmit {
		logrus.Fatalf("Could not find job %s in job config.", *job)
	}

	client := &testClient{
		IsPresubmit: foundPresubmit,
		Presubmit:   presubmit,
		Postsubmit:  postsubmit,

		JobName:   *job,
		RepoOwner: *repoOwner,
		RepoName:  *repoName,
		PRNumber:  *pr,
		Author:    *author,
		BaseRef:   *baseRef,
		BaseSHA:   *baseSHA,
		PullSHA:   *pullSHA,
		Refs:      *refs,

		DryRun: *dryRun,
		Report: *report && !presubmit.SkipReport,

		KubeJob:       kubeJob,
		KubeClient:    kc,
		JenkinsClient: jenkinsClient,
	}
	l := logrus.WithFields(fields(client))
	if foundPresubmit && presubmit.Spec == nil {
		if err := client.TestPRJenkins(); err != nil {
			l.WithError(err).Error("Error testing PR on Jenkins.")
			return
		}
	} else {
		if err := client.TestKubernetes(); err != nil {
			l.WithError(err).Error("Error testing PR on Kubernetes.")
			return
		}
	}
	br, err := buildReq(*repoOwner, *repoName, *author, *refs)
	if err != nil {
		l.WithError(err).Error("Error parsing refs.")
		return
	}
	for _, job := range presubmit.RunAfterSuccess {
		if err := line.StartJob(kc, job.Name, job.Context, br); err != nil {
			l.WithError(err).Error("Error starting child job.")
			return
		}
	}
	for _, job := range postsubmit.RunAfterSuccess {
		if err := line.StartJob(kc, job.Name, "", br); err != nil {
			l.WithError(err).Error("Error starting child job.")
			return
		}
	}
}

func buildReq(org, repo, author, refs string) (line.BuildRequest, error) {
	allRefs := strings.Split(refs, ",")
	branchRef := strings.Split(allRefs[0], ":")
	br := line.BuildRequest{
		Org:     org,
		Repo:    repo,
		BaseRef: branchRef[0],
		BaseSHA: branchRef[1],
	}
	for _, r := range allRefs[1:] {
		pullRef := strings.Split(r, ":")
		n, err := strconv.Atoi(pullRef[0])
		if err != nil {
			return br, err
		}
		br.Pulls = append(br.Pulls, line.Pull{
			Number: n,
			Author: author,
			SHA:    pullRef[1],
		})
	}
	return br, nil
}

func fields(c *testClient) logrus.Fields {
	return logrus.Fields{
		"job":      c.JobName,
		"org":      c.RepoOwner,
		"repo":     c.RepoName,
		"pr":       c.PRNumber,
		"base-ref": c.BaseRef,
		"base-sha": c.BaseSHA,
		"pull-sha": c.PullSHA,
		"refs":     c.Refs,
	}
}

// TestKubernetes starts a pod and watches it, updating GitHub status as
// necessary.
// We modify the pod's spec to have the build parameters such as PR number
// passed in as environment variables. We also include the service account
// secret.
func (c *testClient) TestKubernetes() error {
	logrus.WithFields(fields(c)).Info("Starting pod.")
	buildID := getBuildID(*totURL, c.JobName)
	spec := c.Presubmit.Spec
	if spec == nil {
		spec = c.Postsubmit.Spec
	}
	spec.NodeSelector = map[string]string{
		"role": "build",
	}
	spec.RestartPolicy = "Never"

	// keep this synchronized with get_running_build_log in Gubernator!
	podName := fmt.Sprintf("%s-%s", c.JobName, buildID)
	if len(podName) > 60 {
		podName = podName[len(podName)-60:]
	}

	for i := range spec.Containers {
		spec.Containers[i].Name = fmt.Sprintf("%s-%d", podName, i)
		spec.Containers[i].Env = append(spec.Containers[i].Env,
			kube.EnvVar{
				Name:  "JOB_NAME",
				Value: c.JobName,
			},
			kube.EnvVar{
				Name:  "REPO_OWNER",
				Value: c.RepoOwner,
			},
			kube.EnvVar{
				Name:  "REPO_NAME",
				Value: c.RepoName,
			},
			kube.EnvVar{
				Name:  "PULL_REFS",
				Value: c.Refs,
			},
			kube.EnvVar{
				Name:  "PULL_NUMBER",
				Value: strconv.Itoa(c.PRNumber),
			},
			kube.EnvVar{
				Name:  "PULL_BASE_REF",
				Value: c.BaseRef,
			},
			kube.EnvVar{
				Name:  "PULL_BASE_SHA",
				Value: c.BaseSHA,
			},
			kube.EnvVar{
				Name:  "PULL_PULL_SHA",
				Value: c.PullSHA,
			},
			kube.EnvVar{
				Name:  "BUILD_NUMBER",
				Value: buildID,
			},
			kube.EnvVar{
				Name:  "GOOGLE_APPLICATION_CREDENTIALS",
				Value: "/etc/service-account/service-account.json",
			},
		)
		spec.Containers[i].VolumeMounts = append(spec.Containers[i].VolumeMounts,
			kube.VolumeMount{
				Name:      "service",
				MountPath: "/etc/service-account",
				ReadOnly:  true,
			},
			kube.VolumeMount{
				Name:      "cache-ssd",
				MountPath: "/root/.cache",
			},
		)
		// Set the HostPort to 9999 for all build pods so that they are forced
		// onto different nodes. Once pod affinity is GA, use that instead.
		spec.Containers[i].Ports = append(spec.Containers[i].Ports,
			kube.Port{
				ContainerPort: 9999,
				HostPort:      9999,
			},
		)
	}
	spec.Volumes = append(spec.Volumes,
		kube.Volume{
			Name: "service",
			Secret: &kube.SecretSource{
				Name: "service-account",
			},
		},
		kube.Volume{
			Name: "cache-ssd",
			HostPath: &kube.HostPathSource{
				Path: "/mnt/disks/ssd0",
			},
		},
	)
	p := kube.Pod{
		Metadata: kube.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				"job":  c.JobName,
				"repo": c.RepoName,
			},
		},
		Spec: *spec,
	}
	actual, err := c.KubeClient.CreatePod(p)
	if err != nil {
		c.tryCreateStatus("", github.StatusError, "Error creating build pod.", testInfra)
		return err
	}
	c.setJobAgent(agentKubernetes)

	resultURL := c.guberURL(buildID)
	podName = actual.Metadata.Name
	c.tryCreateStatus(podName, github.StatusPending, "Build started", resultURL)
	for {
		po, err := c.KubeClient.GetPod(actual.Metadata.Name)
		if err != nil {
			c.tryCreateStatus(podName, github.StatusError, "Error waiting for pod to complete.", testInfra)
			return err
		}
		if po.Status.Phase == kube.PodSucceeded {
			c.tryCreateStatus(podName, github.StatusSuccess, "Build succeeded.", resultURL)
			break
		} else if po.Status.Phase == kube.PodFailed {
			c.tryCreateStatus(podName, github.StatusFailure, "Build failed.", resultURL)
			break
		} else if po.Status.Phase == kube.PodUnknown {
			c.tryCreateStatus(podName, github.StatusError, "Error watching build.", resultURL)
			break
		}
		time.Sleep(20 * time.Second)
	}
	return nil
}

// TestPRJenkins starts a Jenkins build and watches it, updating the GitHub
// status as necessary.
func (c *testClient) TestPRJenkins() error {
	if size, err := c.JenkinsClient.QueueSize(); err != nil {
		c.tryCreateStatus("", github.StatusError, "Error checking Jenkins queue.", testInfra)
		return err
	} else if size > 200 {
		c.tryCreateStatus("", github.StatusError, "Jenkins overloaded. Please try again later.", testInfra)
		return nil
	}
	logrus.WithFields(fields(c)).Info("Starting build.")
	c.tryCreateStatus("", github.StatusPending, "Build triggered.", "")
	b, err := c.JenkinsClient.Build(jenkins.BuildRequest{
		JobName: c.Presubmit.Name,
		Number:  c.PRNumber,
		Refs:    c.Refs,
		BaseRef: c.BaseRef,
		BaseSHA: c.BaseSHA,
		PullSHA: c.PullSHA,
	})
	if err != nil {
		c.tryCreateStatus("", github.StatusError, "Error starting build.", testInfra)
		return err
	}
	eq, err := c.JenkinsClient.Enqueued(b)
	if err != nil {
		c.tryCreateStatus("", github.StatusError, "Error queueing build.", testInfra)
		return err
	}
	for eq { // Wait for it to move out of the queue
		time.Sleep(10 * time.Second)
		eq, err = c.JenkinsClient.Enqueued(b)
		if err != nil {
			c.tryCreateStatus("", github.StatusError, "Error in queue.", testInfra)
			return err
		}
	}

	result, err := c.JenkinsClient.Status(b)
	if err != nil {
		c.tryCreateStatus("", github.StatusError, "Error waiting for build.", testInfra)
		return err
	}
	c.setJobAgent(agentJenkins)

	buildID := strconv.Itoa(result.Number)
	buildName := fmt.Sprintf("%s-%s", c.Presubmit.Name, buildID)
	resultURL := c.guberURL(buildID)
	c.tryCreateStatus(buildName, github.StatusPending, "Build started.", resultURL)
	for {
		if err != nil {
			c.tryCreateStatus(buildName, github.StatusError, "Error waiting for build.", testInfra)
			return err
		}
		if result.Building {
			time.Sleep(time.Minute)
		} else {
			if result.Success {
				c.tryCreateStatus(buildName, github.StatusSuccess, "Build succeeded.", resultURL)
				break
			} else {
				c.tryCreateStatus(buildName, github.StatusFailure, "Build failed.", resultURL)
				break
			}
		}
		result, err = c.JenkinsClient.Status(b)
	}
	return nil
}

func (c *testClient) guberURL(build string) string {
	var url string
	if c.IsPresubmit {
		url = guberBasePR
	} else {
		url = guberBasePush
	}
	if c.RepoOwner != "kubernetes" {
		url = fmt.Sprintf("%s/%s_%s", url, c.RepoOwner, c.RepoName)
	} else if c.RepoName != "kubernetes" {
		url = fmt.Sprintf("%s/%s", url, c.RepoName)
	}
	prName := strconv.Itoa(c.PRNumber)
	if prName == "0" {
		prName = "batch"
	}
	if c.IsPresubmit {
		return fmt.Sprintf("%s/%s/%s/%s/", url, prName, c.Presubmit.Name, build)
	} else {
		return fmt.Sprintf("%s/%s/%s/", url, c.Postsubmit.Name, build)
	}
}

func (c *testClient) setJobAgent(agent string) {
	if err := line.SetJobAgent(c.KubeClient, c.KubeJob, agent); err != nil {
		logrus.WithFields(fields(c)).WithError(err).Errorf("Error setting Kube Job agent annotation to %s.", agent)
	}
}

func (c *testClient) tryCreateStatus(podName, state, desc, url string) {
	logrus.WithFields(fields(c)).WithFields(logrus.Fields{
		"state":       state,
		"description": desc,
		"url":         url,
	}).Info("Setting GitHub and Kubernetes status.")
	if c.Report {
		if err := crier.ReportToCrier(*crierURL, crier.Report{
			RepoOwner:    c.RepoOwner,
			RepoName:     c.RepoName,
			Author:       c.Author,
			Number:       c.PRNumber,
			Commit:       c.PullSHA,
			Context:      c.Presubmit.Context,
			State:        state,
			Description:  desc,
			RerunCommand: c.Presubmit.RerunCommand,
			URL:          url,
		}); err != nil {
			logrus.WithFields(fields(c)).WithError(err).Error("Error reporting to crier.")
		}
	}
	if err := line.SetJobStatus(c.KubeClient, podName, c.KubeJob, state, desc, url); err != nil {
		logrus.WithFields(fields(c)).WithError(err).Error("Error setting Kube Job status.")
	}
}

func getKubeJob(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`^job-name="([^"]+)"$`)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		m := re.FindStringSubmatch(scanner.Text())
		if len(m) == 2 {
			return m[1], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("could not find job-name in %s", path)
}
