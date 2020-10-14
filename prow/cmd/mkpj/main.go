/*
Copyright 2017 The Kubernetes Authors.

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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	"sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	pjapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	prowconfig "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

type options struct {
	jobName       string
	configPath    string
	jobConfigPath string
	dryRun        bool
	outputPath    string
	kubeOptions   prowflagutil.KubernetesOptions
	baseRef       string
	baseSha       string
	pullNumber    int
	pullSha       string
	pullAuthor    string
	org           string
	repo          string

	local bool

	github       prowflagutil.GitHubOptions
	githubClient githubClient
	pullRequest  *github.PullRequest
}

func (o *options) genJobSpec(conf *config.Config) (config.JobBase, prowapi.ProwJobSpec) {
	for fullRepoName, ps := range conf.PresubmitsStatic {
		org, repo, err := splitRepoName(fullRepoName)
		if err != nil {
			logrus.WithError(err).Warnf("Invalid repo name %s.", fullRepoName)
			continue
		}
		for _, p := range ps {
			if p.Name == o.jobName {
				return p.JobBase, pjutil.PresubmitSpec(p, prowapi.Refs{
					Org:     org,
					Repo:    repo,
					BaseRef: o.baseRef,
					BaseSHA: o.baseSha,
					Pulls: []prowapi.Pull{{
						Author: o.pullAuthor,
						Number: o.pullNumber,
						SHA:    o.pullSha,
					}},
				})
			}
		}
	}
	for fullRepoName, ps := range conf.PostsubmitsStatic {
		org, repo, err := splitRepoName(fullRepoName)
		if err != nil {
			logrus.WithError(err).Warnf("Invalid repo name %s.", fullRepoName)
			continue
		}
		for _, p := range ps {
			if p.Name == o.jobName {
				return p.JobBase, pjutil.PostsubmitSpec(p, prowapi.Refs{
					Org:     org,
					Repo:    repo,
					BaseRef: o.baseRef,
					BaseSHA: o.baseSha,
				})
			}
		}
	}
	for _, p := range conf.Periodics {
		if p.Name == o.jobName {
			return p.JobBase, pjutil.PeriodicSpec(p)
		}
	}
	return config.JobBase{}, prowapi.ProwJobSpec{}
}

func (o *options) getPullRequest() (*github.PullRequest, error) {
	if o.pullRequest != nil {
		return o.pullRequest, nil
	}
	pr, err := o.githubClient.GetPullRequest(o.org, o.repo, o.pullNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PullRequest from GitHub: %v", err)
	}
	o.pullRequest = pr
	return pr, nil
}

func (o *options) defaultPR(pjs *prowapi.ProwJobSpec) error {
	if pjs.Refs.Pulls[0].Number == 0 {
		fmt.Fprint(os.Stderr, "PR Number: ")
		var pullNumber int
		fmt.Scanln(&pullNumber)
		pjs.Refs.Pulls[0].Number = pullNumber
		o.pullNumber = pullNumber
	}
	if pjs.Refs.Pulls[0].Author == "" {
		pr, err := o.getPullRequest()
		if err != nil {
			return err
		}
		pjs.Refs.Pulls[0].Author = pr.User.Login
	}
	if pjs.Refs.Pulls[0].SHA == "" {
		pr, err := o.getPullRequest()
		if err != nil {
			return err
		}
		pjs.Refs.Pulls[0].SHA = pr.Head.SHA
	}
	return nil
}

func (o *options) defaultBaseRef(pjs *prowapi.ProwJobSpec) error {
	if pjs.Refs.BaseRef == "" {
		if o.pullNumber != 0 {
			pr, err := o.getPullRequest()
			if err != nil {
				return err
			}
			pjs.Refs.BaseRef = pr.Base.Ref
		} else {
			fmt.Fprint(os.Stderr, "Base ref (e.g. master): ")
			fmt.Scanln(&pjs.Refs.BaseRef)
		}
	}
	if pjs.Refs.BaseSHA == "" {
		if o.pullNumber != 0 {
			pr, err := o.getPullRequest()
			if err != nil {
				return err
			}
			pjs.Refs.BaseSHA = pr.Base.SHA
		} else {
			baseSHA, err := o.githubClient.GetRef(o.org, o.repo, fmt.Sprintf("heads/%s", pjs.Refs.BaseRef))
			if err != nil {
				logrus.Fatalf("failed to get base sha: %v", err)
				return err
			}
			pjs.Refs.BaseSHA = baseSHA
		}
	}
	return nil
}

type githubClient interface {
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetRef(org, repo, ref string) (string, error)
}

func (o *options) Validate() error {
	if o.jobName == "" {
		return errors.New("required flag --job was unset")
	}

	if o.configPath == "" {
		return errors.New("required flag --config-path was unset")
	}

	if err := o.github.Validate(false); err != nil {
		return err
	}

	if err := o.kubeOptions.Validate(false); err != nil {
		return err
	}

	return nil
}

func gatherOptions() options {
	var o options
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.jobName, "job", "", "Job to run.")
	fs.BoolVar(&o.local, "local", false, "Print help for running locally")
	fs.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.baseRef, "base-ref", "", "Git base ref under test")
	fs.StringVar(&o.baseSha, "base-sha", "", "Git base SHA under test")
	fs.IntVar(&o.pullNumber, "pull-number", 0, "Git pull number under test")
	fs.StringVar(&o.pullSha, "pull-sha", "", "Git pull SHA under test")
	fs.StringVar(&o.pullAuthor, "pull-author", "", "Git pull author under test")
	fs.BoolVar(&o.dryRun, "dry-run", false, "Executes a dry-run, displaying the job YAML without submitting the job to Prow")
	o.kubeOptions.AddFlags(fs)
	o.github.AddFlags(fs)
	o.github.AllowAnonymous = true
	o.github.AllowDirectAccess = true
	fs.Parse(os.Args[1:])
	return o
}

// getJobArtifactsURL returns the artifacts URL for the given job
func getJobArtifactsURL(prowJob *pjapi.ProwJob, config *prowconfig.Config) string {
	var identifier string
	if prowJob.Spec.Refs != nil {
		identifier = fmt.Sprintf("%s/%s", prowJob.Spec.Refs.Org, prowJob.Spec.Refs.Repo)
	} else {
		identifier = fmt.Sprintf("%s/%s", prowJob.Spec.ExtraRefs[0].Org, prowJob.Spec.ExtraRefs[0].Repo)
	}
	spec := downwardapi.NewJobSpec(prowJob.Spec, prowJob.Status.BuildID, prowJob.Name)
	jobBasePath, _, _ := gcsupload.PathsForJob(config.Plank.GetDefaultDecorationConfigs(identifier).GCSConfiguration, &spec, "")
	return fmt.Sprintf("%s%s/%s",
		config.Deck.Spyglass.GCSBrowserPrefix,
		config.Plank.GetDefaultDecorationConfigs(identifier).GCSConfiguration.Bucket,
		jobBasePath,
	)
}

// Calls toJSON method on a jobResult type and writes it to the output path
func writeResultOutput(prowjobResult JobResult, outputPath string, fileSystem afero.Fs) error {
	j, err := prowjobResult.toJSON()
	if err != nil {
		return fmt.Errorf("unable to marshal prowjob result to JSON: %w", err)
	}

	afs := afero.Afero{Fs: fileSystem}
	if outputPath != "" {
		err = afs.WriteFile(outputPath, j, 0755)
		if err != nil {
			logrus.WithField("output path", outputPath).Error("error writing to output file")
			return err
		}
	} else {
		logrus.Info(string(j))
	}

	return nil
}

type JobResult interface {
	toJSON() ([]byte, error)
}

type prowjobResult struct {
	Status       pjapi.ProwJobState `json:"status"`
	ArtifactsURL string             `json:"prowjob_artifacts_url"`
	URL          string             `json:"prowjob_url"`
}

func (p *prowjobResult) toJSON() ([]byte, error) {
	return json.MarshalIndent(p, "", "    ")
}

func triggerProwJob(o options, prowjob *pjapi.ProwJob, config *prowconfig.Config, envVars map[string]string, fileSystem afero.Fs) error {
	logrus.Info("getting cluster config")
	pjclient, err := o.kubeOptions.ProwJobClient(config.ProwJobNamespace, o.dryRun)
	if err != nil {
		return fmt.Errorf("failed getting prowjob client: %w", err)
	}
	// kubeconfig needs to be set in the KUBECONFIG env variable
	// clusterConfig, err := util.LoadClusterConfig()
	// if err != nil {
	// 	return fmt.Errorf("failed to load cluster configuration: %w", err)
	// }

	// pjcset, err := pjclientset.NewForConfig(clusterConfig)
	// if err != nil {
	// 	return fmt.Errorf("failed to create prowjob clientset: %w", err)
	// }
	// pjclient := pjcset.ProwV1().ProwJobs(config.ProwJobNamespace)

	logrus.WithFields(pjutil.ProwJobFields(prowjob)).Info("submitting a new prowjob")
	created, err := pjclient.Create(context.TODO(), prowjob, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to submit the prowjob: %w", err)
	}

	logger := logrus.WithFields(pjutil.ProwJobFields(created))
	logger.Info("submitted the prowjob, waiting for its result")

	selector := fields.SelectorFromSet(map[string]string{"metadata.name": created.Name})

	for {
		w, err := pjclient.Watch(context.TODO(), metav1.ListOptions{FieldSelector: selector.String()})
		if err != nil {
			return fmt.Errorf("failed to create watch for ProwJobs: %w", err)
		}

		for event := range w.ResultChan() {
			prowJob, ok := event.Object.(*pjapi.ProwJob)
			if !ok {
				return fmt.Errorf("received an unexpected object from Watch: object-type %s", fmt.Sprintf("%T", event.Object))
			}

			prowJobArtifactsURL := getJobArtifactsURL(prowJob, config)

			switch prowJob.Status.State {
			case pjapi.FailureState, pjapi.AbortedState, pjapi.ErrorState:
				pjr := &prowjobResult{
					Status:       prowJob.Status.State,
					ArtifactsURL: prowJobArtifactsURL,
					URL:          prowJob.Status.URL,
				}
				err = writeResultOutput(pjr, o.outputPath, fileSystem)
				if err != nil {
					logrus.Error("Unable to write prowjob result to file")
				}
				logrus.Warn("job failed")
				return nil
			case pjapi.SuccessState:
				pjr := &prowjobResult{
					Status:       prowJob.Status.State,
					ArtifactsURL: prowJobArtifactsURL,
					URL:          prowJob.Status.URL,
				}
				err = writeResultOutput(pjr, o.outputPath, fileSystem)
				if err != nil {
					logrus.Error("Unable to write prowjob result to file")
				}
				logrus.Info("job succeeded")
				return nil
			}
		}
	}
}
func main() {
	o := gatherOptions()
	fileSystem := afero.NewOsFs()
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatalf("Bad flags")
	}

	conf, err := config.Load(o.configPath, o.jobConfigPath)
	if err != nil {
		logrus.WithError(err).Fatal("Error loading config")
	}

	var secretAgent *secret.Agent
	if o.github.TokenPath != "" {
		secretAgent = &secret.Agent{}
		if err := secretAgent.Start([]string{o.github.TokenPath}); err != nil {
			logrus.WithError(err).Fatal("Failed to start secret agent")
		}
	}
	o.githubClient, err = o.github.GitHubClient(secretAgent, false)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to get GitHub client")
	}
	job, pjs := o.genJobSpec(conf)
	if job.Name == "" {
		logrus.Fatalf("Job %s not found.", o.jobName)
	}
	if pjs.Refs != nil {
		o.org = pjs.Refs.Org
		o.repo = pjs.Refs.Repo
		if len(pjs.Refs.Pulls) != 0 {
			if err := o.defaultPR(&pjs); err != nil {
				logrus.WithError(err).Fatal("Failed to default PR")
			}
		}
		if err := o.defaultBaseRef(&pjs); err != nil {
			logrus.WithError(err).Fatal("Failed to default base ref")
		}
	}
	pj := pjutil.NewProwJob(pjs, job.Labels, job.Annotations)
	b, err := yaml.Marshal(&pj)
	if err != nil {
		logrus.WithError(err).Fatal("Error marshalling YAML.")
	}
	fmt.Print(string(b))
	if o.local {
		logrus.Info("Use 'bazel run //prow/cmd/phaino' to run this job locally in docker")
	}
	if o.dryRun {
		os.Exit(0)
	}
	if err := triggerProwJob(o, &pj, conf, nil, fileSystem); err != nil {
		logrus.WithError(err).Fatalf("failed while submitting job or watching its result")
	}
}

func splitRepoName(repo string) (string, string, error) {
	s := strings.SplitN(repo, "/", 2)
	if len(s) != 2 {
		return "", "", fmt.Errorf("repo %s cannot be split into org/repo", repo)
	}
	return s[0], s[1], nil
}
