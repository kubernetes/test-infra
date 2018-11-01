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

// Package reporter contains helpers for publishing statues to Pub
// statuses in Github.
package reporter

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/pubsub"

	"k8s.io/test-infra/prow/kube"
)

const (
	pubsubProjectLabel = "prow.k8s.io/pubsub-project"
	pubsubTopicLabel   = "prow.k8s.io/pubsub-topic"
	pubsubRunIDLabel   = "prow.k8s.io/pubsub-runID"
)

// ReportMessage is a message structure used to pass a prowjob status to Pub/Sub topic.s
type ReportMessage struct {
	Project string            `json:"project"`
	Topic   string            `json:"topic"`
	RunID   string            `json:"runid"`
	Status  kube.ProwJobState `json:"status"`
	URL     string            `json:"url"`
}

// Client is a reporter client fed to crier controller
type Client struct {
	// Empty structure because unlike github or gerrit client, one GCP Pub/Sub client is tied to one GCP project.
	// While GCP project name is provided by the label in each prowjob.
	// Which means we could create a Pub/Sub client only when we actually get a prowjob to do reporting,
	// instead of creating a Pub/Sub client while initializing the reporter client.
}

// NewReporter creates a new Pub/Sub reporter
func NewReporter() *Client {
	return &Client{}
}

// GetName returns the name of the reporter
func (c *Client) GetName() string {
	return "pubsub-reporter"
}

// ShouldReport tells if a prowjob should be reported by this reporter
func (c *Client) ShouldReport(pj *kube.ProwJob) bool {
	return pj.Labels[pubsubProjectLabel] != "" && pj.Labels[pubsubTopicLabel] != ""
}

// Report takes a prowjob, and generate a pubsub ReportMessage and publish to specific Pub/Sub topic
// based on Pub/Sub related labels if they exist in this prowjob
func (c *Client) Report(pj *kube.ProwJob) error {
	message := generateMessageFromPJ(pj)

	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, message.Project)

	if err != nil {
		return fmt.Errorf("could not create pubsub Client: %v", err)
	}
	topic := client.Topic(message.Topic)

	d, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("could not marshal pubsub report: %v", err)
	}

	res := topic.Publish(ctx, &pubsub.Message{
		Data: d,
	})

	_, err = res.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to publish pubsub message: %v", err)
	}

	return nil
}

func generateMessageFromPJ(pj *kube.ProwJob) *ReportMessage {
	projectName := pj.Labels[pubsubProjectLabel]
	topicName := pj.Labels[pubsubTopicLabel]
	runID := pj.GetLabels()[pubsubRunIDLabel]

	psReport := &ReportMessage{
		Project: projectName,
		Topic:   topicName,
		RunID:   runID,
		Status:  pj.Status.State,
		URL:     pj.Status.URL,
	}

	return psReport
}
