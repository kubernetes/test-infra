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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	slackclient "k8s.io/test-infra/prow/slack"
)

const reporterName = "slackreporter"

type slackClient interface {
	WriteMessage(text, channel string) error
}

type slackReporter struct {
	client slackClient
	config func(*prowapi.Refs) config.SlackReporter
	dryRun bool
}

func (sr *slackReporter) getConfig(pj *v1.ProwJob) config.SlackReporter {
	refs := pj.Spec.Refs
	if refs == nil && len(pj.Spec.ExtraRefs) > 0 {
		refs = &pj.Spec.ExtraRefs[0]
	}
	return sr.config(refs)
}

func jobConfig(pj *v1.ProwJob) *v1.SlackReporterConfig {
	if pj.Spec.ReporterConfig != nil {
		return pj.Spec.ReporterConfig.Slack
	}
	return nil
}

func channel(prowCfg config.SlackReporter, jobCfg *v1.SlackReporterConfig) string {
	if jobCfg != nil && jobCfg.Channel != "" {
		return jobCfg.Channel
	}
	return prowCfg.Channel
}

func reportTemplate(prowCfg config.SlackReporter, jobCfg *v1.SlackReporterConfig) string {
	if jobCfg != nil && jobCfg.ReportTemplate != "" {
		return jobCfg.ReportTemplate
	}
	return prowCfg.ReportTemplate
}

func (sr *slackReporter) Report(log *logrus.Entry, pj *v1.ProwJob) ([]*v1.ProwJob, *reconcile.Result, error) {
	return []*v1.ProwJob{pj}, nil, sr.report(log, pj)
}

func (sr *slackReporter) report(log *logrus.Entry, pj *v1.ProwJob) error {
	prowCfg := sr.getConfig(pj)
	jobCfg := jobConfig(pj)
	templateStr := reportTemplate(prowCfg, jobCfg)
	channel := channel(prowCfg, jobCfg)
	b := &bytes.Buffer{}
	tmpl, err := template.New("").Parse(templateStr)
	if err != nil {
		log.WithError(err).Error("failed to parse template")
		return fmt.Errorf("failed to parse template: %v", err)
	}
	if err := tmpl.Execute(b, pj); err != nil {
		log.WithError(err).Error("failed to execute report template")
		return fmt.Errorf("failed to execute report template: %v", err)
	}
	if sr.dryRun {
		log.WithField("messagetext", b.String()).Debug("Skipping reporting because dry-run is enabled")
		return nil
	}
	if err := sr.client.WriteMessage(b.String(), channel); err != nil {
		log.WithError(err).Error("failed to write Slack message")
		return fmt.Errorf("failed to write Slack message: %v", err)
	}
	return nil
}

func (sr *slackReporter) GetName() string {
	return reporterName
}

func (sr *slackReporter) ShouldReport(logger *logrus.Entry, pj *v1.ProwJob) bool {
	jobCfg := jobConfig(pj)
	prowCfg := sr.getConfig(pj)

	// The job needs to be reported, if its type has a match with the
	// JobTypesToReport in the Prow config.
	typeShouldReport := false
	for _, typeToReport := range prowCfg.JobTypesToReport {
		if typeToReport == pj.Spec.Type {
			typeShouldReport = true
			break
		}
	}

	// If a user specifically put a channel on their job, they want
	// it to be reported regardless of the job types setting.
	jobShouldReport := false
	if jobCfg != nil && jobCfg.Channel != "" {
		jobShouldReport = true
	}

	// The job should only be reported if its state has a match with the
	// JobStatesToReport config.
	// Note the JobStatesToReport configured in the Prow job can overwrite the
	// Prow config.
	jobStatesToReport := prowCfg.JobStatesToReport
	if jobCfg != nil && len(jobCfg.JobStatesToReport) != 0 {
		jobStatesToReport = jobCfg.JobStatesToReport
	}
	stateShouldReport := false
	for _, stateToReport := range jobStatesToReport {
		if pj.Status.State == stateToReport {
			stateShouldReport = true
			break
		}
	}

	shouldReport := stateShouldReport && (typeShouldReport || jobShouldReport)
	logger.WithField("reporting", shouldReport).Debug("Determined should report")
	return shouldReport
}

func New(cfg func(refs *prowapi.Refs) config.SlackReporter, dryRun bool, token func() []byte) *slackReporter {
	return &slackReporter{
		client: slackclient.NewClient(token),
		config: cfg,
		dryRun: dryRun,
	}
}
