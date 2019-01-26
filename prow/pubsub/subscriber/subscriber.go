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
	"encoding/json"
	"fmt"

	"cloud.google.com/go/pubsub"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

const (
	prowEventType        = "prow.k8s.io/pubsub.EventType"
	periodicProwJobEvent = "prow.k8s.io/pubsub.PeriodicProwJobEvent"
)

// PeriodicProwJobEvent contains the minimum information required to start a ProwJob.
type PeriodicProwJobEvent struct {
	Name        string            `json:"name"`
	Envs        map[string]string `json:"envs,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// FromPayload set the PeriodicProwJobEvent from the PubSub message payload.
func (pe *PeriodicProwJobEvent) FromPayload(data []byte) error {
	if err := json.Unmarshal(data, pe); err != nil {
		return err
	}
	return nil
}

// ToMessage generates a PubSub Message from a PeriodicProwJobEvent.
func (pe *PeriodicProwJobEvent) ToMessage() (*pubsub.Message, error) {
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

// KubeClientInterface mostly for testing.
type KubeClientInterface interface {
	CreateProwJob(job *kube.ProwJob) (*kube.ProwJob, error)
}

// Subscriber handles Pub/Sub subscriptions, update metrics,
// validates them using Prow Configuration and
// use a KubeClientInterface to create Prow Jobs.
type Subscriber struct {
	ConfigAgent *config.Agent
	Metrics     *Metrics
	KubeClient  KubeClientInterface
	Reporter    reportClient
}

type messageInterface interface {
	getAttributes() map[string]string
	getPayload() []byte
	getID() string
	ack()
	nack()
}

type reportClient interface {
	Report(pj *v1.ProwJob) error
	ShouldReport(pj *v1.ProwJob) bool
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

func extractFromAttribute(attrs map[string]string, key string) (string, error) {
	value, ok := attrs[key]
	if !ok {
		return "", fmt.Errorf("unable to find %s from the attributes", key)
	}
	return value, nil
}

func (s *Subscriber) handleMessage(msg messageInterface, subscription string) error {
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
	switch eType {
	case periodicProwJobEvent:
		err := s.handlePeriodicJob(l, msg, subscription)
		if err != nil {
			l.WithError(err).Error("failed to create Prow Periodic Job")
			s.Metrics.ErrorCounter.With(prometheus.Labels{subscriptionLabel: subscription})
		}
		return err
	}
	err = fmt.Errorf("unsupported event type")
	l.WithError(err).Error("failed to read message")
	s.Metrics.ErrorCounter.With(prometheus.Labels{subscriptionLabel: subscription})
	return err
}

func (s *Subscriber) handlePeriodicJob(l *logrus.Entry, msg messageInterface, subscription string) error {

	var pe PeriodicProwJobEvent
	var prowJob kube.ProwJob

	reportProwJobFailure := func(pj *kube.ProwJob, err error) {
		pj.Status.State = kube.ErrorState
		pj.Status.Description = err.Error()
		if s.Reporter.ShouldReport(&prowJob) {
			s.Reporter.Report(&prowJob)
		}
	}

	if err := pe.FromPayload(msg.getPayload()); err != nil {
		return err
	}
	var periodicJob *config.Periodic
	l.Infof("looking for periodic job %s", pe.Name)
	for _, job := range s.ConfigAgent.Config().AllPeriodics() {
		if job.Name == pe.Name {
			periodicJob = &job
			break
		}
	}
	if periodicJob == nil {
		err := fmt.Errorf("failed to find associated periodic job %s", pe.Name)
		l.WithError(err).Errorf("failed to create job %s", pe.Name)
		prowJob = pjutil.NewProwJobWithAnnotation(kube.ProwJobSpec{}, nil, pe.Annotations)
		reportProwJobFailure(&prowJob, err)
		return err
	}
	prowJobSpec := pjutil.PeriodicSpec(*periodicJob)
	// Adds / Updates Labels from prow job event
	for k, v := range pe.Labels {
		periodicJob.Labels[k] = v
	}

	// Adds annotations
	prowJob = pjutil.NewProwJobWithAnnotation(prowJobSpec, periodicJob.Labels, pe.Annotations)
	// Adds / Updates Environments to containers
	if prowJob.Spec.PodSpec != nil {
		for _, c := range prowJob.Spec.PodSpec.Containers {
			for k, v := range pe.Envs {
				c.Env = append(c.Env, kube.EnvVar{Name: k, Value: v})
			}
		}
	}

	if _, err := s.KubeClient.CreateProwJob(&prowJob); err != nil {
		l.WithError(err).Errorf("failed to create job %s as %s", pe.Name, prowJob.Name)
		reportProwJobFailure(&prowJob, err)
		return err
	}
	l.Infof("periodic job %s created as %s", pe.Name, prowJob.Name)
	return nil
}
