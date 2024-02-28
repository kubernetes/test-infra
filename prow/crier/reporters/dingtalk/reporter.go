package dingtalk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"

	"github.com/sirupsen/logrus"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	dingtalkclient "k8s.io/test-infra/prow/dingtalk"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	reporterName = "dingTalk-reporter"
)

type dingTalkClient interface {
	WriteMessage(msg, token string) error
}

type dingTalkReporter struct {
	client dingTalkClient
	config func(*prowapi.Refs) config.DingTalkReporter
	dryRun bool
}

func (sr *dingTalkReporter) getConfig(pj *prowapi.ProwJob) (*config.DingTalkReporter, *prowapi.DingTalkReporterConfig) {
	refs := pj.Spec.Refs
	if refs == nil && len(pj.Spec.ExtraRefs) > 0 {
		refs = &pj.Spec.ExtraRefs[0]
	}
	globalConfig := sr.config(refs)
	var jobDingTalkConfig *prowapi.DingTalkReporterConfig
	if pj.Spec.ReporterConfig != nil && pj.Spec.ReporterConfig.DingTalk != nil {
		jobDingTalkConfig = pj.Spec.ReporterConfig.DingTalk
	}
	return &globalConfig, jobDingTalkConfig
}

func (sr *dingTalkReporter) Report(_ context.Context, log *logrus.Entry, pj *prowapi.ProwJob) ([]*prowapi.ProwJob, *reconcile.Result, error) {
	return []*prowapi.ProwJob{pj}, nil, sr.report(log, pj)
}

func (sr *dingTalkReporter) report(log *logrus.Entry, pj *prowapi.ProwJob) error {
	globalDingTalkConfig, jobDingTalkConfig := sr.getConfig(pj)
	if globalDingTalkConfig != nil {
		jobDingTalkConfig = jobDingTalkConfig.ApplyDefault(&globalDingTalkConfig.DingTalkReporterConfig)
	}
	if jobDingTalkConfig == nil {
		return errors.New("resolved dingTalk config is empty") // Shouldn't happen at all, just in case
	}

	b := &bytes.Buffer{}
	tmpl, err := template.New("").Parse(jobDingTalkConfig.ReportTemplate)
	if err != nil {
		log.WithError(err).Error("failed to parse template")
		return fmt.Errorf("failed to parse template: %w", err)
	}
	if err := tmpl.Execute(b, pj); err != nil {
		log.WithError(err).Error("failed to execute report template")
		return fmt.Errorf("failed to execute report template: %w", err)
	}
	if sr.dryRun {
		log.WithField("messagejson", b.String()).Debug("Skipping reporting because dry-run is enabled")
		return nil
	}
	if err := sr.client.WriteMessage(b.String(), jobDingTalkConfig.Token); err != nil {
		log.WithError(err).Error("failed to write DingTalk message")
		return fmt.Errorf("failed to write DingTalk message: %w", err)
	}
	return nil
}

func (sr *dingTalkReporter) GetName() string {
	return reporterName
}

func (sr *dingTalkReporter) ShouldReport(_ context.Context, logger *logrus.Entry, pj *prowapi.ProwJob) bool {
	globalDingTalkConfig, jobDingTalkConfig := sr.getConfig(pj)

	var typeShouldReport bool
	if globalDingTalkConfig.JobTypesToReport != nil {
		for _, tp := range globalDingTalkConfig.JobTypesToReport {
			if tp == pj.Spec.Type {
				typeShouldReport = true
				break
			}
		}
	}

	// If a user specifically put a token on their job, they want
	// it to be reported regardless of the job types setting.
	var jobShouldReport bool
	if jobDingTalkConfig != nil && jobDingTalkConfig.Token != "" {
		jobShouldReport = true
	}

	// The job should only be reported if its state has a match with the
	// JobStatesToReport config.
	// Note the JobStatesToReport configured in the Prow job can overwrite the
	// Prow config.
	var stateShouldReport bool
	if merged := jobDingTalkConfig.ApplyDefault(&globalDingTalkConfig.DingTalkReporterConfig); merged != nil && merged.JobStatesToReport != nil {
		if merged.Report != nil && !*merged.Report {
			logger.WithField("job_states_to_report", merged.JobStatesToReport).Debug("Skip dingTalk reporting as 'report: false', could result from 'job_states_to_report: []'.")
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

func New(cfg func(refs *prowapi.Refs) config.DingTalkReporter, dryRun bool) *dingTalkReporter {
	return &dingTalkReporter{
		client: dingtalkclient.NewClient(),
		config: cfg,
		dryRun: dryRun,
	}
}
