/*
Copyright 2022 The Kubernetes Authors.

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

package fakepubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"k8s.io/test-infra/prow/pubsub/subscriber"
)

type PubSubMessageForSub struct {
	Attributes map[string]string
	Data       subscriber.ProwJobEvent
}

type Client struct {
	projectID    string
	pubsubClient *pubsub.Client
}

func NewClient(projectID, pubsubEmulatorHost string) (*Client, error) {
	client, err := newClientForEmulator(projectID, pubsubEmulatorHost)
	if err != nil {
		return nil, fmt.Errorf("Unable to create pubsub client to project %q for the emulator: %v", projectID, err)
	}

	return &Client{
		projectID:    projectID,
		pubsubClient: client,
	}, nil
}

// newClientForEmulator returns a pubsub client that is hardcoded to always talk
// to the fakepubsub service running in the test KIND cluster via the
// pubsubEmulatorHost parameter. This is taken from
// https://github.com/googleapis/google-cloud-go/blob/e43c095c94e44a95c618861f9da8f2469b53be16/pubsub/pubsub.go#L126.
// This is better than getting the PUBSUB_EMULATOR_HOST environment variable
// because this makes the code thread-safe (we no longer rely on a global
// environment variable).
func newClientForEmulator(projectID, pubsubEmulatorHost string) (*pubsub.Client, error) {
	conn, err := grpc.Dial(pubsubEmulatorHost, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc.Dial: %v", err)
	}
	o := []option.ClientOption{option.WithGRPCConn(conn)}
	o = append(o, option.WithTelemetryDisabled())
	return pubsub.NewClientWithConfig(context.Background(), projectID, nil, o...)
}

// PublishMessage creates a Pub/Sub message that sub understands (to create a
// ProwJob). The podName parameter is used by the integration tests;
// specifically, each test case invocation generates a UUID which is used as the
// name of the ProwJob CR. Then when the test pod is created, it is also named
// with the same UUID. This makes checking for the creation of jobs and pods
// very easy in the tests.
func (c *Client) PublishMessage(ctx context.Context, msg PubSubMessageForSub, topicID string) error {
	bytes, err := json.Marshal(msg.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal: %v", err)
	}

	t := c.pubsubClient.Topic(topicID)
	result := t.Publish(ctx, &pubsub.Message{Data: bytes, Attributes: msg.Attributes})

	id, err := result.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to publish: %v", err)
	}

	logrus.Infof("successfully published message %v; msg ID: %v", string(bytes), id)

	return nil
}

// CreateSubscription creates a Pub/Sub topic and a corresponding subscription.
func (c *Client) CreateSubscription(ctx context.Context, projectID, topicID, subscriptionID string) error {
	topic, err := c.pubsubClient.CreateTopic(ctx, topicID)
	if err != nil {
		return err
	}

	if _, err := c.pubsubClient.CreateSubscription(ctx, subscriptionID, pubsub.SubscriptionConfig{
		Topic:            topic,
		AckDeadline:      10 * time.Second,
		ExpirationPolicy: 25 * time.Hour,
	}); err != nil {
		return err
	}

	return nil
}
