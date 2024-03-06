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
	"fmt"
	"strings"

	"cloud.google.com/go/pubsub"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	prowcrd "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gangway"
	"k8s.io/test-infra/prow/kube"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ProwEventType          = "prow.k8s.io/pubsub.EventType"
	PeriodicProwJobEvent   = "prow.k8s.io/pubsub.PeriodicProwJobEvent"
	PresubmitProwJobEvent  = "prow.k8s.io/pubsub.PresubmitProwJobEvent"
	PostsubmitProwJobEvent = "prow.k8s.io/pubsub.PostsubmitProwJobEvent"
)

// ProwJobEvent contains the minimum information required to start a ProwJob.
type ProwJobEvent struct {
	Name string `json:"name"`
	// Refs are used by presubmit and postsubmit jobs supplying baseSHA and SHA
	Refs        *prowcrd.Refs     `json:"refs,omitempty"`
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
	return pe.ToMessageOfType(PeriodicProwJobEvent)
}

// ToMessage generates a PubSub Message from a ProwJobEvent.
func (pe *ProwJobEvent) ToMessageOfType(t string) (*pubsub.Message, error) {
	data, err := json.Marshal(pe)
	if err != nil {
		return nil, err
	}
	message := pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			ProwEventType: t,
		},
	}
	return &message, nil
}

// Subscriber handles Pub/Sub subscriptions, update metrics,
// validates them using Prow Configuration and
// use a ProwJobClient to create Prow Jobs.
type Subscriber struct {
	ConfigAgent        *config.Agent
	Metrics            *Metrics
	ProwJobClient      gangway.ProwJobClient
	Reporter           reportClient
	InRepoConfigGetter config.InRepoConfigGetter
}

type messageInterface interface {
	getAttributes() map[string]string
	getPayload() []byte
	getID() string
	ack()
	nack()
}

type reportClient interface {
	Report(ctx context.Context, log *logrus.Entry, pj *prowcrd.ProwJob) ([]*prowcrd.ProwJob, *reconcile.Result, error)
	ShouldReport(ctx context.Context, log *logrus.Entry, pj *prowcrd.ProwJob) bool
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
		return "", fmt.Errorf("unable to find %q from the attributes", key)
	}
	return value, nil
}

func (s *Subscriber) getReporterFunc(l *logrus.Entry) gangway.ReporterFunc {
	return func(pj *prowcrd.ProwJob, state prowcrd.ProwJobState, err error) {
		pj.Status.State = state
		pj.Status.Description = "Successfully triggered prowjob."
		if err != nil {
			pj.Status.Description = fmt.Sprintf("Failed creating prowjob: %v", err)
		}
		if s.Reporter.ShouldReport(context.TODO(), l, pj) {
			if _, _, err := s.Reporter.Report(context.TODO(), l, pj); err != nil {
				l.WithError(err).Warning("Failed to report status.")
			}
		}
	}
}

func (s *Subscriber) handleMessage(msg messageInterface, subscription string, allowedClusters []string) error {

	msgID := msg.getID()
	l := logrus.WithFields(logrus.Fields{
		"pubsub-subscription": subscription,
		"pubsub-id":           msgID})

	// First, convert the incoming message into a CreateJobExecutionRequest type.
	cjer, err := s.msgToCjer(l, msg, subscription)
	if err != nil {
		return err
	}

	// Do not check for HTTP client authorization, because we're handling a
	// PubSub message.
	var allowedApiClient *config.AllowedApiClient = nil
	var requireTenantID bool = false

	cfgAdapter := gangway.ProwCfgAdapter{Config: s.ConfigAgent.Config()}
	if _, err = gangway.HandleProwJob(l, s.getReporterFunc(l), cjer, s.ProwJobClient, &cfgAdapter, s.InRepoConfigGetter, allowedApiClient, requireTenantID, allowedClusters); err != nil {
		l.WithError(err).Info("failed to create Prow Job")
		s.Metrics.ErrorCounter.With(prometheus.Labels{
			subscriptionLabel: subscription,
			// This should be the only case prow operator should pay more
			// attention too, because errors here are more likely caused by
			// prow. (There are exceptions, which we can iterate slightly later)
			errorTypeLabel: "failed-handle-prowjob",
		}).Inc()
	}

	// TODO(chaodaiG): debugging purpose, remove once done debugging.
	l.WithField("payload", string(msg.getPayload())).WithField("post-id", msg.getID()).Debug("Finished handling message")
	return err
}

// msgToCjer converts an incoming message (PubSub message) into a CJER. It
// actually does 2 conversions --- from the message to ProwJobEvent (in order to
// unmarshal the raw bytes) then again from ProwJobEvent to a CJER.
func (s *Subscriber) msgToCjer(l *logrus.Entry, msg messageInterface, subscription string) (*gangway.CreateJobExecutionRequest, error) {
	msgAttributes := msg.getAttributes()
	msgPayload := msg.getPayload()

	l.WithField("payload", string(msgPayload)).Debug("Received message")
	s.Metrics.MessageCounter.With(prometheus.Labels{subscriptionLabel: subscription}).Inc()

	// Note that a CreateJobExecutionRequest is a superset of ProwJobEvent.
	// However we still use ProwJobEvent here because we want to use the
	// existing jobHandlers to fetch the prowJobSpec (and the jobHandlers expect
	// a ProwJobEvent as an argument).
	var pe ProwJobEvent

	// We use ProwJobEvent here mainly to ensrue that the incoming payload
	// (JSON) is well-formed. We convert it into a CreateJobExecutionRequest
	// type here and never use it anywhere else.
	l.WithField("raw-payload", string(msgPayload)).Debug("Raw payload passed in handleProwJob.")
	if err := pe.FromPayload(msgPayload); err != nil {
		return nil, err
	}

	eType, err := extractFromAttribute(msgAttributes, ProwEventType)
	if err != nil {
		l.WithError(err).Error("failed to read message")
		s.Metrics.ErrorCounter.With(prometheus.Labels{
			subscriptionLabel: subscription,
			errorTypeLabel:    "malformed-message",
		}).Inc()
		return nil, err
	}

	return s.peToCjer(l, &pe, eType, subscription)
}

func (s *Subscriber) peToCjer(l *logrus.Entry, pe *ProwJobEvent, eType, subscription string) (*gangway.CreateJobExecutionRequest, error) {

	cjer := gangway.CreateJobExecutionRequest{
		JobName: strings.TrimSpace(pe.Name),
	}

	// First encode the job type.
	switch eType {
	case PeriodicProwJobEvent:
		cjer.JobExecutionType = gangway.JobExecutionType_PERIODIC
	case PresubmitProwJobEvent:
		cjer.JobExecutionType = gangway.JobExecutionType_PRESUBMIT
	case PostsubmitProwJobEvent:
		cjer.JobExecutionType = gangway.JobExecutionType_POSTSUBMIT
	default:
		l.WithField("type", eType).Info("Unsupported event type")
		s.Metrics.ErrorCounter.With(prometheus.Labels{
			subscriptionLabel: subscription,
			errorTypeLabel:    "unsupported-event-type",
		}).Inc()
		return nil, fmt.Errorf("unsupported event type: %s", eType)
	}

	pso := gangway.PodSpecOptions{}
	pso.Labels = make(map[string]string)
	for k, v := range pe.Labels {
		pso.Labels[k] = v
	}

	pso.Annotations = make(map[string]string)
	for k, v := range pe.Annotations {
		pso.Annotations[k] = v
	}

	pso.Envs = make(map[string]string)
	for k, v := range pe.Envs {
		pso.Envs[k] = v
	}

	cjer.PodSpecOptions = &pso

	var err error

	if pe.Refs != nil {
		cjer.Refs, err = gangway.FromCrdRefs(pe.Refs)
		if err != nil {
			return nil, err
		}

		// Add "https://" prefix to orgRepo if this is a gerrit job.
		// (Unfortunately gerrit jobs use the full repo URL as the identifier.)
		prefix := "https://"
		if pso.Labels[kube.GerritRevision] != "" && !strings.HasPrefix(cjer.Refs.Org, prefix) {
			cjer.Refs.Org = prefix + cjer.Refs.Org
		}
	}

	return &cjer, nil
}
