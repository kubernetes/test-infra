/*
Copyright 2018 The Kubernetes Authors.

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

package subscriber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"cloud.google.com/go/pubsub"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/pjutil"
)

const (
	prowEventType          = "prow.k8s.io/pubsub.EventType"
	periodicProwJobEvent   = "prow.k8s.io/pubsub.PeriodicProwJobEvent"
	presubmitProwJobEvent  = "prow.k8s.io/pubsub.PresubmitProwJobEvent"
	postsubmitProwJobEvent = "prow.k8s.io/pubsub.PostsubmitProwJobEvent"
)

// Ensure interface is intact
var _ prowCfgClient = (*config.Config)(nil)

// prowCfgClient is for unit test purpose
type prowCfgClient interface {
	AllPeriodics() []config.Periodic
	GetPresubmits(gc git.ClientFactory, identifier string, baseSHAGetter config.RefGetter, headSHAGetters ...config.RefGetter) ([]config.Presubmit, error)
	GetPresubmitsStatic(identifier string) []config.Presubmit
}

// ProwJobEvent contains the minimum information required to start a ProwJob.
type ProwJobEvent struct {
	Name string `json:"name"`
	// Refs are used by presubmit and postsubmit jobs supplying baseSHA and SHA
	Refs        *v1.Refs          `json:"refs,omitempty"`
	Envs        map[string]string `json:"envs,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// FromPayload set the ProwJobEvent from the PubSub message payload.
func (pe *ProwJobEvent) FromPayload(data []byte) error {
	if err := json.Unmarshal(data, pe); err != nil {
		return err
	}
	return nil
}

// ToMessage generates a PubSub Message from a ProwJobEvent.
func (pe *ProwJobEvent) ToMessage() (*pubsub.Message, error) {
	data, err := json.Marshal(pe)
	if err != nil {
		return nil, err
	}
	message := pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			prowEventType: periodicProwJobEvent,
		},
	}
	return &message, nil
}

// ProwJobClient mostly for testing.
type ProwJobClient interface {
	Create(context.Context, *prowapi.ProwJob, metav1.CreateOptions) (*prowapi.ProwJob, error)
}

// Subscriber handles Pub/Sub subscriptions, update metrics,
// validates them using Prow Configuration and
// use a ProwJobClient to create Prow Jobs.
type Subscriber struct {
	ConfigAgent   *config.Agent
	Metrics       *Metrics
	ProwJobClient ProwJobClient
	GitClient     git.ClientFactory
	Reporter      reportClient
}

type messageInterface interface {
	getAttributes() map[string]string
	getPayload() []byte
	getID() string
	ack()
	nack()
}

type reportClient interface {
	Report(ctx context.Context, log *logrus.Entry, pj *prowapi.ProwJob) ([]*prowapi.ProwJob, *reconcile.Result, error)
	ShouldReport(ctx context.Context, log *logrus.Entry, pj *prowapi.ProwJob) bool
}

type pubSubMessage struct {
	pubsub.Message
}

func (m *pubSubMessage) getAttributes() map[string]string {
	return m.Attributes
}

func (m *pubSubMessage) getPayload() []byte {
	return m.Data
}

func (m *pubSubMessage) getID() string {
	return m.ID
}

func (m *pubSubMessage) ack() {
	m.Message.Ack()
}
func (m *pubSubMessage) nack() {
	m.Message.Nack()
}

// jobHandler handles job type specific logic
type jobHandler interface {
	getProwJobSpec(cfg prowCfgClient, pe ProwJobEvent) (*v1.ProwJobSpec, map[string]string, error)
}

// periodicJobHandler implements jobHandler
type periodicJobHandler struct{}

func (peh *periodicJobHandler) getProwJobSpec(cfg prowCfgClient, pe ProwJobEvent) (*v1.ProwJobSpec, map[string]string, error) {
	var periodicJob *config.Periodic
	// TODO(chaodaiG): do we want to support inrepoconfig when
	// https://github.com/kubernetes/test-infra/issues/21729 is done?
	for _, job := range cfg.AllPeriodics() {
		if job.Name == pe.Name {
			periodicJob = &job
			break
		}
	}
	if periodicJob == nil {
		return nil, nil, fmt.Errorf("failed to find associated periodic job %q", pe.Name)
	}

	prowJobSpec := pjutil.PeriodicSpec(*periodicJob)
	return &prowJobSpec, periodicJob.Labels, nil
}

// presubmitJobHandler implements jobHandler
type presubmitJobHandler struct {
	GitClient git.ClientFactory
}

func (prh *presubmitJobHandler) getProwJobSpec(cfg prowCfgClient, pe ProwJobEvent) (*v1.ProwJobSpec, map[string]string, error) {
	// presubmit jobs require Refs and Refs.Pulls to be set
	refs := pe.Refs
	if refs == nil {
		return nil, nil, errors.New("Refs must be supplied")
	}
	if len(refs.Pulls) == 0 {
		return nil, nil, errors.New("at least 1 Pulls is required")
	}

	var presubmitJob *config.Presubmit
	org, repo, branch := refs.Org, refs.Repo, refs.BaseRef
	orgRepo := org + "/" + repo
	baseSHAGetter := func() (string, error) {
		return refs.BaseSHA, nil
	}
	var headSHAGetters []func() (string, error)
	for _, pull := range refs.Pulls {
		pull := pull
		headSHAGetters = append(headSHAGetters, func() (string, error) {
			return pull.SHA, nil
		})
	}

	// This will work with inrepoconfig
	presubmits, err := cfg.GetPresubmits(prh.GitClient, orgRepo, baseSHAGetter, headSHAGetters...)
	if err != nil {
		// Fall back to static presubmits to avoid deadlocking when a presubmit is used to verify
		// inrepoconfig
		logrus.WithError(err).Debug("Failed to get presubmits")
		presubmits = cfg.GetPresubmitsStatic(orgRepo)
	}

	for _, job := range presubmits {
		if !job.CouldRun(branch) { // filter out jobs that are not branch matching
			continue
		}
		if job.Name == pe.Name {
			if presubmitJob != nil {
				return nil, nil, fmt.Errorf("%s matches multiple prow jobs", pe.Name)
			}
			presubmitJob = &job
		}
	}
	if presubmitJob == nil {
		return nil, nil, fmt.Errorf("failed to find associated periodic job %q", pe.Name)
	}

	prowJobSpec := pjutil.PresubmitSpec(*presubmitJob, *refs)
	return &prowJobSpec, presubmitJob.Labels, nil
}

// ppostsubmitJobHandler implements jobHandler
type postsubmitJobHandler struct{}

func (poh *postsubmitJobHandler) getProwJobSpec(cfg prowCfgClient, pe ProwJobEvent) (*v1.ProwJobSpec, map[string]string, error) {
	return nil, nil, errors.New("postsubmit not supported yet")
}

func extractFromAttribute(attrs map[string]string, key string) (string, error) {
	value, ok := attrs[key]
	if !ok {
		return "", fmt.Errorf("unable to find %q from the attributes", key)
	}
	return value, nil
}

func (s *Subscriber) handleMessage(msg messageInterface, subscription string, allowedClusters []string) error {
	l := logrus.WithFields(logrus.Fields{
		"pubsub-subscription": subscription,
		"pubsub-id":           msg.getID()})
	s.Metrics.MessageCounter.With(prometheus.Labels{subscriptionLabel: subscription}).Inc()
	l.Info("Received message")
	eType, err := extractFromAttribute(msg.getAttributes(), prowEventType)
	if err != nil {
		l.WithError(err).Error("failed to read message")
		s.Metrics.ErrorCounter.With(prometheus.Labels{subscriptionLabel: subscription})
		return err
	}

	var jh jobHandler
	switch eType {
	case periodicProwJobEvent:
		jh = &periodicJobHandler{}
	case presubmitProwJobEvent:
		jh = &presubmitJobHandler{GitClient: s.GitClient}
	case postsubmitProwJobEvent:
		jh = &postsubmitJobHandler{}
	default:
		l.WithField("type", eType).Error("Unsupported event type")
		s.Metrics.ErrorCounter.With(prometheus.Labels{subscriptionLabel: subscription})
		return fmt.Errorf("unsupported event type: %s", eType)
	}
	if err = s.handleProwJob(l, jh, msg, subscription, allowedClusters); err != nil {
		l.WithError(err).Error("failed to create Prow Job")
		s.Metrics.ErrorCounter.With(prometheus.Labels{subscriptionLabel: subscription})
	}
	return err
}

func (s *Subscriber) handleProwJob(l *logrus.Entry, jh jobHandler, msg messageInterface, subscription string, allowedClusters []string) error {

	var pe ProwJobEvent
	var prowJob prowapi.ProwJob

	if err := pe.FromPayload(msg.getPayload()); err != nil {
		return err
	}

	reportProwJobFailure := func(pj *prowapi.ProwJob, err error) {
		pj.Status.State = prowapi.ErrorState
		pj.Status.Description = err.Error()
		if s.Reporter.ShouldReport(context.TODO(), l, pj) {
			if _, _, err := s.Reporter.Report(context.TODO(), l, pj); err != nil {
				l.Warningf("failed to report status. %v", err)
			}
		}
	}

	prowJobSpec, labels, err := jh.getProwJobSpec(s.ConfigAgent.Config(), pe)
	if err != nil {
		l.WithError(err).Errorf("failed to create job %q", pe.Name)
		prowJob = pjutil.NewProwJob(prowapi.ProwJobSpec{}, nil, pe.Annotations)
		reportProwJobFailure(&prowJob, err)
		return err
	}
	if prowJobSpec == nil {
		return fmt.Errorf("failed getting prowjob spec") // This should not happen
	}

	// deny job that runs on not allowed cluster
	var clusterIsAllowed bool
	for _, allowedCluster := range allowedClusters {
		if allowedCluster == "*" || allowedCluster == prowJobSpec.Cluster {
			clusterIsAllowed = true
			break
		}
	}
	if !clusterIsAllowed {
		err := fmt.Errorf("cluster %s is not allowed. Can be fixed by defining this cluster under pubsub_triggers -> allowed_clusters", prowJobSpec.Cluster)
		l.WithField("cluster", prowJobSpec.Cluster).Warn("cluster not allowed")
		prowJob = pjutil.NewProwJob(*prowJobSpec, nil, pe.Annotations)
		reportProwJobFailure(&prowJob, err)
		return err
	}

	// Adds / Updates Labels from prow job event
	for k, v := range pe.Labels {
		labels[k] = v
	}

	// Adds annotations
	prowJob = pjutil.NewProwJob(*prowJobSpec, labels, pe.Annotations)
	// Adds / Updates Environments to containers
	if prowJob.Spec.PodSpec != nil {
		for i, c := range prowJob.Spec.PodSpec.Containers {
			for k, v := range pe.Envs {
				c.Env = append(c.Env, coreapi.EnvVar{Name: k, Value: v})
			}
			prowJob.Spec.PodSpec.Containers[i].Env = c.Env
		}
	}

	if _, err := s.ProwJobClient.Create(context.TODO(), &prowJob, metav1.CreateOptions{}); err != nil {
		l.WithError(err).Errorf("failed to create job %q as %q", pe.Name, prowJob.Name)
		reportProwJobFailure(&prowJob, err)
		return err
	}
	l.Infof("Job %q created as %q", pe.Name, prowJob.Name)
	return nil
}
