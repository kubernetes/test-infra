/*
Copyright 2019 The Kubernetes Authors.

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

package slack

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/pjutil"
	slackclient "k8s.io/test-infra/prow/slack"
)

const reporterName = "slackreporter"

type slackReporter struct {
	client *slackclient.Client
	config func(*prowapi.Refs) config.SlackReporter
	logger *logrus.Entry
	dryRun bool
}

func jobChannel(pj *v1.ProwJob) (string, bool) {
	if pj.Spec.ReporterConfig != nil && pj.Spec.ReporterConfig.Slack != nil && pj.Spec.ReporterConfig.Slack.Channel != "" {
		return pj.Spec.ReporterConfig.Slack.Channel, true
	}
	return "", false
}

func channel(cfg config.SlackReporter, pj *v1.ProwJob) string {
	if channel, set := jobChannel(pj); set {
		return channel
	}
	return cfg.Channel
}

func (sr *slackReporter) Report(pj *v1.ProwJob) ([]*v1.ProwJob, error) {
	config := sr.config(pj.Spec.Refs)
	channel := channel(config, pj)
	b := &bytes.Buffer{}
	tmpl, err := template.New("").Parse(config.ReportTemplate)
	if err != nil {
		sr.logger.WithField("prowjob", pj.Name).Errorf("failed to parse template: %v", err)
		return nil, fmt.Errorf("failed to parse template: %v", err)
	}
	if err := tmpl.Execute(b, pj); err != nil {
		sr.logger.WithField("prowjob", pj.Name).WithError(err).Error("failed to execute report template")
		return nil, fmt.Errorf("failed to execute report template: %v", err)
	}
	if sr.dryRun {
		sr.logger.
			WithField("prowjob", pj.Name).
			WithField("messagetext", b.String()).
			Debug("Skipping reporting because dry-run is enabled")
		return []*v1.ProwJob{pj}, nil
	}
	if err := sr.client.WriteMessage(b.String(), channel); err != nil {
		sr.logger.WithError(err).Error("failed to write Slack message")
		return nil, fmt.Errorf("failed to write Slack message: %v", err)
	}
	return []*v1.ProwJob{pj}, nil
}

func (sr *slackReporter) GetName() string {
	return reporterName
}

func (sr *slackReporter) ShouldReport(pj *v1.ProwJob) bool {
	logger := sr.logger.WithFields(pjutil.ProwJobFields(pj))
	// if a user specifically put a channel on their job, they want
	// it to be reported regardless of what other settings exist
	if _, set := jobChannel(pj); set {
		logger.Debugf("reporting as channel is explicitly set")
		return true
	}
	config := sr.config(pj.Spec.Refs)

	stateShouldReport := false
	for _, stateToReport := range config.JobStatesToReport {
		if pj.Status.State == stateToReport {
			stateShouldReport = true
			break
		}
	}

	typeShouldReport := false
	for _, typeToReport := range config.JobTypesToReport {
		if typeToReport == pj.Spec.Type {
			typeShouldReport = true
			break
		}
	}

	logger.Debugf("reporting=%t", stateShouldReport && typeShouldReport)
	return stateShouldReport && typeShouldReport
}

func New(cfg func(refs *prowapi.Refs) config.SlackReporter, dryRun bool, token func() []byte) *slackReporter {
	return &slackReporter{
		client: slackclient.NewClient(token),
		config: cfg,
		logger: logrus.WithField("component", reporterName),
		dryRun: dryRun,
	}
}
