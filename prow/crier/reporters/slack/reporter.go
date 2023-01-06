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
	"context"
	"errors"
	"fmt"
	"text/template"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	slackclient "k8s.io/test-infra/prow/slack"
)

const (
	reporterName    = "slackreporter"
	DefaultHostName = "*"
)

type slackClient interface {
	WriteMessage(text, channel string) error
}

type slackReporter struct {
	clients map[string]slackClient
	config  func(*prowapi.Refs) config.SlackReporter
	dryRun  bool
}

func hostAndChannel(cfg *v1.SlackReporterConfig) (string, string) {
	host, channel := cfg.Host, cfg.Channel
	if host == "" {
		host = DefaultHostName
	}
	return host, channel
}

func (sr *slackReporter) getConfig(pj *v1.ProwJob) (*config.SlackReporter, *v1.SlackReporterConfig) {
	refs := pj.Spec.Refs
	if refs == nil && len(pj.Spec.ExtraRefs) > 0 {
		refs = &pj.Spec.ExtraRefs[0]
	}
	globalConfig := sr.config(refs)
	var jobSlackConfig *v1.SlackReporterConfig
	if pj.Spec.ReporterConfig != nil && pj.Spec.ReporterConfig.Slack != nil {
		jobSlackConfig = pj.Spec.ReporterConfig.Slack
	}
	return &globalConfig, jobSlackConfig
}

func (sr *slackReporter) Report(_ context.Context, log *logrus.Entry, pj *v1.ProwJob) ([]*v1.ProwJob, *reconcile.Result, error) {
	return []*v1.ProwJob{pj}, nil, sr.report(log, pj)
}

func (sr *slackReporter) report(log *logrus.Entry, pj *v1.ProwJob) error {
	globalSlackConfig, jobSlackConfig := sr.getConfig(pj)
	if globalSlackConfig != nil {
		jobSlackConfig = jobSlackConfig.ApplyDefault(&globalSlackConfig.SlackReporterConfig)
	}
	if jobSlackConfig == nil {
		return errors.New("resolved slack config is empty") // Shouldn't happen at all, just in case
	}
	host, channel := hostAndChannel(jobSlackConfig)

	client, ok := sr.clients[host]
	if !ok {
		return fmt.Errorf("host '%s' not supported", host)
	}
	b := &bytes.Buffer{}
	tmpl, err := template.New("").Parse(jobSlackConfig.ReportTemplate)
	if err != nil {
		log.WithError(err).Error("failed to parse template")
		return fmt.Errorf("failed to parse template: %w", err)
	}
	if err := tmpl.Execute(b, pj); err != nil {
		log.WithError(err).Error("failed to execute report template")
		return fmt.Errorf("failed to execute report template: %w", err)
	}
	if sr.dryRun {
		log.WithField("messagetext", b.String()).Debug("Skipping reporting because dry-run is enabled")
		return nil
	}
	if err := client.WriteMessage(b.String(), channel); err != nil {
		log.WithError(err).Error("failed to write Slack message")
		return fmt.Errorf("failed to write Slack message: %w", err)
	}
	return nil
}

func (sr *slackReporter) GetName() string {
	return reporterName
}

func (sr *slackReporter) ShouldReport(_ context.Context, logger *logrus.Entry, pj *v1.ProwJob) bool {
	globalSlackConfig, jobSlackConfig := sr.getConfig(pj)

	var typeShouldReport bool
	if globalSlackConfig.JobTypesToReport != nil {
		for _, tp := range globalSlackConfig.JobTypesToReport {
			if tp == pj.Spec.Type {
				typeShouldReport = true
				break
			}
		}
	}

	// If a user specifically put a channel on their job, they want
	// it to be reported regardless of the job types setting.
	var jobShouldReport bool
	if jobSlackConfig != nil && jobSlackConfig.Channel != "" {
		jobShouldReport = true
	}

	// The job should only be reported if its state has a match with the
	// JobStatesToReport config.
	// Note the JobStatesToReport configured in the Prow job can overwrite the
	// Prow config.
	var stateShouldReport bool
	if merged := jobSlackConfig.ApplyDefault(&globalSlackConfig.SlackReporterConfig); merged != nil && merged.JobStatesToReport != nil {
		if merged.Report != nil && !*merged.Report {
			logger.WithField("job_states_to_report", merged.JobStatesToReport).Debug("Skip slack reporting as 'report: false', could result from 'job_states_to_report: []'.")
			return false
		}
		for _, stateToReport := range merged.JobStatesToReport {
			if pj.Status.State == stateToReport {
				stateShouldReport = true
				break
			}
		}
	}

	shouldReport := stateShouldReport && (typeShouldReport || jobShouldReport)
	logger.WithField("reporting", shouldReport).Debug("Determined should report")
	return shouldReport
}

func New(cfg func(refs *prowapi.Refs) config.SlackReporter, dryRun bool, tokensMap map[string]func() []byte) *slackReporter {
	clients := map[string]slackClient{}
	for key, val := range tokensMap {
		clients[key] = slackclient.NewClient(val)
	}
	return &slackReporter{
		clients: clients,
		config:  cfg,
		dryRun:  dryRun,
	}
}
