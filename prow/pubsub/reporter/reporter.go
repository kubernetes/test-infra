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
	"strings"

	"cloud.google.com/go/pubsub"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

const (
	// PubSubProjectLabel annotation
	PubSubProjectLabel = "prow.k8s.io/pubsub.project"
	// PubSubTopicLabel annotation
	PubSubTopicLabel = "prow.k8s.io/pubsub.topic"
	// PubSubRunIDLabel annotation
	PubSubRunIDLabel = "prow.k8s.io/pubsub.runID"
	// GCSPrefix is the prefix for a gcs path
	GCSPrefix = "gs://"
)

// ReportMessage is a message structure used to pass a prowjob status to Pub/Sub topic.s
type ReportMessage struct {
	Project string            `json:"project"`
	Topic   string            `json:"topic"`
	RunID   string            `json:"runid"`
	Status  kube.ProwJobState `json:"status"`
	URL     string            `json:"url"`
	GCSPath string            `json:"gcs_path"`
}

// Client is a reporter client fed to crier controller
type Client struct {
	config config.Getter
}

// NewReporter creates a new Pub/Sub reporter
func NewReporter(cfg config.Getter) *Client {
	return &Client{
		config: cfg,
	}
}

// GetName returns the name of the reporter
func (c *Client) GetName() string {
	return "pubsub-reporter"
}

func findLabels(pj *kube.ProwJob, labels ...string) map[string]string {
	// Support checking for both labels(deprecated) and annotations(new) for backward compatibility
	pubSubMap := map[string]string{}
	for _, label := range labels {
		if pj.Annotations[label] != "" {
			pubSubMap[label] = pj.Annotations[label]
		} else {
			pubSubMap[label] = pj.Labels[label]
		}
	}
	return pubSubMap
}

// ShouldReport tells if a prowjob should be reported by this reporter
func (c *Client) ShouldReport(pj *kube.ProwJob) bool {
	pubSubMap := findLabels(pj, PubSubProjectLabel, PubSubTopicLabel)
	return pubSubMap[PubSubProjectLabel] != "" && pubSubMap[PubSubTopicLabel] != ""
}

// Report takes a prowjob, and generate a pubsub ReportMessage and publish to specific Pub/Sub topic
// based on Pub/Sub related labels if they exist in this prowjob
func (c *Client) Report(pj *kube.ProwJob) error {
	message := c.generateMessageFromPJ(pj)

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

func (c *Client) generateMessageFromPJ(pj *kube.ProwJob) *ReportMessage {
	pubSubMap := findLabels(pj, PubSubProjectLabel, PubSubTopicLabel, PubSubRunIDLabel)
	return &ReportMessage{
		Project: pubSubMap[PubSubProjectLabel],
		Topic:   pubSubMap[PubSubTopicLabel],
		RunID:   pubSubMap[PubSubRunIDLabel],
		Status:  pj.Status.State,
		URL:     pj.Status.URL,
		GCSPath: strings.Replace(pj.Status.URL, c.config().Plank.JobURLPrefix, GCSPrefix, 1),
	}
}
