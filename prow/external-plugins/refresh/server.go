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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"text/template"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/report"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp"
)

const pluginName = "refresh"

var refreshRe = regexp.MustCompile(`(?mi)^/refresh\s*$`)

func helpProvider(_ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: `The refresh plugin is used for refreshing status contexts in PRs. Useful in case GitHub breaks down.`,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/refresh",
		Description: "Refresh status contexts on a PR.",
		WhoCanUse:   "Anyone",
		Examples:    []string{"/refresh"},
	})
	return pluginHelp, nil
}

type server struct {
	tokenGenerator func() []byte
	prowURL        string
	configAgent    *config.Agent
	ghc            github.Client
	log            *logrus.Entry
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok, _ := github.ValidateWebhook(w, r, s.tokenGenerator)
	if !ok {
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")

	if err := s.handleEvent(eventType, eventGUID, payload); err != nil {
		logrus.WithError(err).Error("Error parsing event.")
	}
}

func (s *server) handleEvent(eventType, eventGUID string, payload []byte) error {
	l := logrus.WithFields(
		logrus.Fields{
			"event-type":     eventType,
			github.EventGUID: eventGUID,
		},
	)

	switch eventType {
	case "issue_comment":
		var ic github.IssueCommentEvent
		if err := json.Unmarshal(payload, &ic); err != nil {
			return err
		}
		go func() {
			if err := s.handleIssueComment(l, ic); err != nil {
				s.log.WithError(err).WithFields(l.Data).Info("Refreshing github statuses failed.")
			}
		}()
	default:
		logrus.Debugf("skipping event of type %q", eventType)
	}
	return nil
}

func (s *server) handleIssueComment(l *logrus.Entry, ic github.IssueCommentEvent) error {
	if !ic.Issue.IsPullRequest() || ic.Action != github.IssueCommentActionCreated || ic.Issue.State == "closed" {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	num := ic.Issue.Number

	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  org,
		github.RepoLogField: repo,
		github.PrLogField:   num,
	})

	if !refreshRe.MatchString(ic.Comment.Body) {
		return nil
	}
	s.log.WithFields(l.Data).Info("Requested a status refresh.")

	// TODO: Retries
	resp, err := http.Get(s.prowURL + "/prowjobs.js")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("status code not 2XX: %v", resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var list struct {
		PJs []prowapi.ProwJob `json:"items"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("cannot unmarshal data from deck: %v", err)
	}

	pr, err := s.ghc.GetPullRequest(org, repo, num)
	if err != nil {
		return err
	}

	var presubmits []prowapi.ProwJob
	for _, pj := range list.PJs {
		if pj.Spec.Type != "presubmit" {
			continue
		}
		if !pj.Spec.Report {
			continue
		}
		if pj.Spec.Refs.Pulls[0].Number != num {
			continue
		}
		if pj.Spec.Refs.Pulls[0].SHA != pr.Head.SHA {
			continue
		}
		presubmits = append(presubmits, pj)
	}

	if len(presubmits) == 0 {
		s.log.WithFields(l.Data).Info("No prowjobs found.")
		return nil
	}

	jenkinsConfig := s.configAgent.Config().JenkinsOperators
	kubeReport := s.configAgent.Config().Plank.ReportTemplateForRepo(&prowapi.Refs{Org: org, Repo: repo})
	reportTypes := s.configAgent.Config().GitHubReporter.JobTypesToReport
	for _, pj := range pjutil.GetLatestProwJobs(presubmits, prowapi.PresubmitJob) {
		var reportTemplate *template.Template
		switch pj.Spec.Agent {
		case prowapi.KubernetesAgent:
			reportTemplate = kubeReport
		case prowapi.JenkinsAgent:
			reportTemplate = s.reportForProwJob(pj, jenkinsConfig)
		}
		if reportTemplate == nil {
			continue
		}

		s.log.WithFields(l.Data).Infof("Refreshing the status of job %q (pj: %s)", pj.Spec.Job, pj.ObjectMeta.Name)
		if err := report.Report(s.ghc, reportTemplate, pj, reportTypes); err != nil {
			s.log.WithError(err).WithFields(l.Data).Info("Failed report.")
		}
	}
	return nil
}

func (s *server) reportForProwJob(pj prowapi.ProwJob, configs []config.JenkinsOperator) *template.Template {
	for _, cfg := range configs {
		if cfg.LabelSelector.Matches(labels.Set(pj.Labels)) {
			return cfg.ReportTemplateForRepo(pj.Spec.Refs)
		}
	}
	return nil
}
