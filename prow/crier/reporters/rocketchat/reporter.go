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

package rocketchat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"text/template"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	rocketchatclient "k8s.io/test-infra/prow/rocketchat"
)

const (
	reporterName    = "rocketchatreporter"
	DefaultHostName = "*"
)

type rocketChatClient interface {
	WriteMessage(text, channel string) error
}

type rocketChatReporter struct {
	clients map[string]rocketChatClient
	config  func(*prowapi.Refs) config.RocketChatReporter
	dryRun  bool
}

func hostAndChannel(cfg *prowapi.RocketChatReporterConfig) (string, string) {
	host, channel := cfg.Host, cfg.Channel
	if host == "" {
		host = DefaultHostName
	}
	return host, channel
}

func (rr *rocketChatReporter) getConfig(pj *prowapi.ProwJob) (*config.RocketChatReporter, *prowapi.RocketChatReporterConfig) {
	refs := pj.Spec.Refs
	if refs == nil && len(pj.Spec.ExtraRefs) > 0 {
		refs = &pj.Spec.ExtraRefs[0]
	}
	globalConfig := rr.config(refs)
	var jobRocketChatConfig *prowapi.RocketChatReporterConfig
	if pj.Spec.ReporterConfig != nil && pj.Spec.ReporterConfig.RocketChat != nil {
		jobRocketChatConfig = pj.Spec.ReporterConfig.RocketChat
	}
	return &globalConfig, jobRocketChatConfig
}

func (rr *rocketChatReporter) Report(_ context.Context, log *logrus.Entry, pj *prowapi.ProwJob) ([]*prowapi.ProwJob, *reconcile.Result, error) {
	return []*prowapi.ProwJob{pj}, nil, rr.report(log, pj)
}

func (rr *rocketChatReporter) report(log *logrus.Entry, pj *prowapi.ProwJob) error {
	globalRocketChatConfig, jobRocketChatConfig := rr.getConfig(pj)
	if globalRocketChatConfig != nil {
		jobRocketChatConfig = jobRocketChatConfig.ApplyDefault(&globalRocketChatConfig.RocketChatReporterConfig)
	}
	if jobRocketChatConfig == nil {
		return errors.New("resolved rocketchat config is empty") // Shouldn't happen at all, just in case
	}
	host, channel := hostAndChannel(jobRocketChatConfig)

	client, ok := rr.clients[host]
	if !ok {
		return fmt.Errorf("host '%s' not supported", host)
	}
	b := &bytes.Buffer{}
	tmpl, err := template.New("").Parse(jobRocketChatConfig.ReportTemplate)
	if err != nil {
		log.WithError(err).Error("failed to parse template")
		return fmt.Errorf("failed to parse template: %w", err)
	}
	if err := tmpl.Execute(b, pj); err != nil {
		log.WithError(err).Error("failed to execute report template")
		return fmt.Errorf("failed to execute report template: %w", err)
	}
	if rr.dryRun {
		log.WithField("messagetext", b.String()).Debug("Skipping reporting because dry-run is enabled")
		return nil
	}
	if err := client.WriteMessage(b.String(), channel); err != nil {
		log.WithError(err).Error("failed to write RocketChat message")
		return fmt.Errorf("failed to write RocketChat message: %w", err)
	}
	return nil
}

func (rr *rocketChatReporter) GetName() string {
	return reporterName
}

func (rr *rocketChatReporter) ShouldReport(_ context.Context, logger *logrus.Entry, pj *prowapi.ProwJob) bool {
	globalRocketChatConfig, jobRocketChatConfig := rr.getConfig(pj)

	var typeShouldReport bool
	if globalRocketChatConfig.JobTypesToReport != nil {
		for _, tp := range globalRocketChatConfig.JobTypesToReport {
			if tp == pj.Spec.Type {
				typeShouldReport = true
				break
			}
		}
	}

	// If a user specifically put a channel on their job, they want
	// it to be reported regardless of the job types setting.
	var jobShouldReport bool
	if jobRocketChatConfig != nil && jobRocketChatConfig.Channel != "" {
		jobShouldReport = true
	}

	// The job should only be reported if its state has a match with the
	// JobStatesToReport config.
	// Note the JobStatesToReport configured in the Prow job can overwrite the
	// Prow config.
	var stateShouldReport bool
	if merged := jobRocketChatConfig.ApplyDefault(&globalRocketChatConfig.RocketChatReporterConfig); merged != nil && merged.JobStatesToReport != nil {
		if merged.Report != nil && !*merged.Report {
			logger.WithField("job_states_to_report", merged.JobStatesToReport).Debug("Skip rocketchat reporting as 'report: false', could result from 'job_states_to_report: []'.")
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

func New(cfg func(refs *prowapi.Refs) config.RocketChatReporter, dryRun bool, tokensMap map[string]func() []byte) *rocketChatReporter {
	clients := map[string]rocketChatClient{}
	for key, val := range tokensMap {
		clients[key] = rocketchatclient.NewClient(val)
	}
	return &rocketChatReporter{
		clients: clients,
		config:  cfg,
		dryRun:  dryRun,
	}
}
