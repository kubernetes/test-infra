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

package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	webhookclient "k8s.io/test-infra/prow/webhook"
)

const (
	reporterName = "webhookreporter"
)

type webhookClient interface {
	Send(ctx context.Context, url string, message any) error
}

type webhookReporter struct {
	client webhookClient
	config func(*prowapi.Refs) config.WebhookReporter
	dryRun bool
}

func (sr *webhookReporter) getConfig(pj *v1.ProwJob) (*config.WebhookReporter, *v1.WebhookReporterConfig) {
	refs := pj.Spec.Refs
	if refs == nil && len(pj.Spec.ExtraRefs) > 0 {
		refs = &pj.Spec.ExtraRefs[0]
	}
	globalConfig := sr.config(refs)
	var jobWebhookConfig *v1.WebhookReporterConfig
	if pj.Spec.ReporterConfig != nil && pj.Spec.ReporterConfig.Webhook != nil {
		jobWebhookConfig = pj.Spec.ReporterConfig.Webhook
	}
	return &globalConfig, jobWebhookConfig
}

func (sr *webhookReporter) Report(ctx context.Context, log *logrus.Entry, pj *v1.ProwJob) ([]*v1.ProwJob, *reconcile.Result, error) {
	return []*v1.ProwJob{pj}, nil, sr.report(ctx, log, pj)
}

func (sr *webhookReporter) report(ctx context.Context, log *logrus.Entry, pj *v1.ProwJob) error {
	globalWebhookConfig, jobWebhookConfig := sr.getConfig(pj)
	if globalWebhookConfig != nil {
		jobWebhookConfig = jobWebhookConfig.ApplyDefault(&globalWebhookConfig.WebhookReporterConfig)
	}
	if jobWebhookConfig == nil {
		return errors.New("resolved webhook config is empty") // Shouldn't happen at all, just in case
	}

	payload := struct {
		ProwJob *v1.ProwJob `json:"prow_job"`
	}{
		ProwJob: pj,
	}

	if sr.dryRun {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to JSON-encode the Prow job definition: %w", err)
		}
		log.WithField("messagetext", string(b)).Debug("Skipping reporting because dry-run is enabled")
		return nil
	}
	if err := sr.client.Send(ctx, jobWebhookConfig.URL, payload); err != nil {
		log.WithError(err).Error("failed to send the webhook")
		return fmt.Errorf("failed to send the webhook: %w", err)
	}
	return nil
}

func (*webhookReporter) GetName() string {
	return reporterName
}

func (sr *webhookReporter) ShouldReport(_ context.Context, logger *logrus.Entry, pj *v1.ProwJob) bool {
	globalWebhookConfig, jobWebhookConfig := sr.getConfig(pj)

	var typeShouldReport bool
	if globalWebhookConfig.JobTypesToReport != nil {
		for _, tp := range globalWebhookConfig.JobTypesToReport {
			if tp == pj.Spec.Type {
				typeShouldReport = true
				break
			}
		}
	}

	// If a user specifically put a channel on their job, they want
	// it to be reported regardless of the job types setting.
	var jobShouldReport bool
	if jobWebhookConfig != nil && jobWebhookConfig.URL != "" {
		jobShouldReport = true
	}

	// The job should only be reported if its state has a match with the
	// JobStatesToReport config.
	// Note the JobStatesToReport configured in the Prow job can overwrite the
	// Prow config.
	var stateShouldReport bool
	if merged := jobWebhookConfig.ApplyDefault(&globalWebhookConfig.WebhookReporterConfig); merged != nil && merged.JobStatesToReport != nil {
		if merged.Report != nil && !*merged.Report {
			logger.WithField("job_states_to_report", merged.JobStatesToReport).Debug("Skip webhook reporting as 'report: false', could result from 'job_states_to_report: []'.")
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

func New(cfg func(refs *prowapi.Refs) config.WebhookReporter, dryRun bool, tokenFunc func(message []byte) (string, error)) *webhookReporter {
	return &webhookReporter{
		client: webhookclient.NewClient(tokenFunc),
		config: cfg,
		dryRun: dryRun,
	}
}
