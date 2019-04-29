package reporter

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"text/template"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	slackclient "k8s.io/test-infra/prow/slack"
)

const reporterName = "slackreporter"

type slackReporter struct {
	client *slackclient.Client
	config config.SlackReporter
	logger *logrus.Entry
	dryRun bool
}

func (sr *slackReporter) Report(pj *v1.ProwJob) ([]*v1.ProwJob, error) {
	b := &bytes.Buffer{}
	tmpl, err := template.New("").Parse(sr.config.ReportTemplate)
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
	if err := sr.client.WriteMessage(b.String(), sr.config.Channel); err != nil {
		sr.logger.WithError(err).Error("failed to write Slack message")
		return nil, fmt.Errorf("failed to write Slack message: %v", err)
	}
	return []*v1.ProwJob{pj}, nil
}

func (sr *slackReporter) GetName() string {
	return reporterName
}

func (sr *slackReporter) ShouldReport(pj *v1.ProwJob) bool {

	stateShouldReport := false
	for _, stateToReport := range sr.config.JobStatesToReport {
		if pj.Status.State == stateToReport {
			stateShouldReport = true
			break
		}
	}

	typeShouldReport := false
	for _, typeToReport := range sr.config.JobTypesToReport {
		if typeToReport == pj.Spec.Type {
			typeShouldReport = true
			break
		}
	}

	sr.logger.WithField("prowjob", pj.Name).
		Debugf("reporting=%t", stateShouldReport && typeShouldReport)
	return stateShouldReport && typeShouldReport
}

func New(cfg config.SlackReporter, dryRun bool, tokenFile string) (*slackReporter, error) {
	token, err := ioutil.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read -token-file: %v", err)
	}

	return &slackReporter{
		client: slackclient.NewClient(func() []byte { return token }),
		config: cfg,
		logger: logrus.WithField("component", reporterName),
		dryRun: dryRun,
	}, nil
}
