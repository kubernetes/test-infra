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
	"errors"
	"reflect"
	"strings"

	"github.com/sirupsen/logrus"

	"cloud.google.com/go/pubsub"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
	"k8s.io/test-infra/prow/config"
)

type configToWatch struct {
	config.PubSubTriggers
	config.PubsubSubscriptions
}

// PullServer listen to Pull Pub/Sub subscriptions and handle them.
type PullServer struct {
	Subscriber *Subscriber
	Client     pubsubClientInterface
}

// NewPullServer creates a new PullServer
func NewPullServer(s *Subscriber) *PullServer {
	return &PullServer{
		Subscriber: s,
		Client:     &pubSubClient{},
	}
}

// For testing
type subscriptionInterface interface {
	string() string
	receive(ctx context.Context, f func(context.Context, messageInterface)) error
}

// pubsubClientInterface interfaces with Cloud Pub/Sub client for testing reason
type pubsubClientInterface interface {
	new(ctx context.Context, project string) (pubsubClientInterface, error)
	subscription(id string, maxOutstandingMessages int) subscriptionInterface
}

// pubSubClient is used to interface with a new Cloud Pub/Sub Client
type pubSubClient struct {
	client *pubsub.Client
}

type pubSubSubscription struct {
	sub *pubsub.Subscription
}

func (s *pubSubSubscription) string() string {
	return s.sub.String()
}

func (s *pubSubSubscription) receive(ctx context.Context, f func(context.Context, messageInterface)) error {
	g := func(ctx2 context.Context, msg2 *pubsub.Message) {
		f(ctx2, &pubSubMessage{Message: *msg2})
	}
	return s.sub.Receive(ctx, g)
}

// New creates new Cloud Pub/Sub Client
func (c *pubSubClient) new(ctx context.Context, project string) (pubsubClientInterface, error) {
	client, err := pubsub.NewClient(ctx, project)
	if err != nil {
		return nil, err
	}
	c.client = client
	return c, nil
}

// Subscription creates a reference to an existing subscription via the Cloud Pub/Sub Client.
func (c *pubSubClient) subscription(id string, maxOutstandingMessages int) subscriptionInterface {
	sub := c.client.Subscription(id)
	sub.ReceiveSettings.MaxOutstandingMessages = maxOutstandingMessages
	// Without this setting, a single Receiver can occupy more than the number of `MaxOutstandingMessages`,
	// and other replicas of sub will have nothing to work on.
	// cjwagner and chaodaiG understand it might not make much sense to set both MaxOutstandingMessages
	// and Synchronous, nor did the GoDoc https://github.com/googleapis/google-cloud-go/blob/22ffc18e522c0f943db57f8c943e7356067bedfd/pubsub/subscription.go#L501
	// agrees clearly with us, but trust us, both are required for making sure that every replica has something to do
	sub.ReceiveSettings.Synchronous = true
	return &pubSubSubscription{
		sub: sub,
	}
}

// handlePulls pull for Pub/Sub subscriptions and handle them.
func (s *PullServer) handlePulls(ctx context.Context, projectSubscriptions config.PubSubTriggers) (*errgroup.Group, context.Context, error) {
	// Since config might change we need be able to cancel the current run
	errGroup, derivedCtx := errgroup.WithContext(ctx)
	for _, topics := range projectSubscriptions {
		project, subscriptions, allowedClusters := topics.Project, topics.Topics, topics.AllowedClusters
		client, err := s.Client.new(ctx, project)
		if err != nil {
			return errGroup, derivedCtx, err
		}
		for _, subName := range subscriptions {
			sub := client.subscription(subName, topics.MaxOutstandingMessages)
			logger := logrus.WithFields(logrus.Fields{
				"subscription": sub.string(),
				"project":      project,
			})
			errGroup.Go(func() error {
				logger.Info("Listening for subscription")
				defer logger.Warn("Stopped Listening for subscription")
				err := sub.receive(derivedCtx, func(ctx context.Context, msg messageInterface) {
					if err = s.Subscriber.handleMessage(msg, sub.string(), allowedClusters); err != nil {
						s.Subscriber.Metrics.ACKMessageCounter.With(prometheus.Labels{subscriptionLabel: sub.string()}).Inc()
					} else {
						s.Subscriber.Metrics.NACKMessageCounter.With(prometheus.Labels{subscriptionLabel: sub.string()}).Inc()
					}
					msg.ack()
				})
				if err != nil {
					if errors.Is(derivedCtx.Err(), context.Canceled) {
						logger.WithError(err).Debug("Exiting as context cancelled")
						return nil
					}
					if strings.Contains(err.Error(), "code = PermissionDenied") {
						logger.WithError(err).Warn("Seems like missing permission.")
						return nil
					}
					logger.WithError(err).Error("Failed to listen for subscription")
					return err
				}
				return nil
			})
		}
	}
	return errGroup, derivedCtx, nil
}

// Run will block listening to all subscriptions and return once the context is cancelled
// or one of the subscription has a unrecoverable error.
func (s *PullServer) Run(ctx context.Context) error {
	configEvent := make(chan config.Delta, 2)
	s.Subscriber.ConfigAgent.Subscribe(configEvent)

	var err error
	defer func() {
		if err != nil {
			logrus.WithError(ctx.Err()).Error("Pull server shutting down.")
		}
		logrus.Debug("Pull server shutting down.")
	}()
	currentConfig := configToWatch{
		s.Subscriber.ConfigAgent.Config().PubSubTriggers,
		s.Subscriber.ConfigAgent.Config().PubSubSubscriptions,
	}
	errGroup, derivedCtx, err := s.handlePulls(ctx, currentConfig.PubSubTriggers)
	if err != nil {
		return err
	}

	for {
		select {
		// Parent context. Shutdown
		case <-ctx.Done():
			return nil
		// Current thread context, it may be failing already
		case <-derivedCtx.Done():
			err = errGroup.Wait()
			return err
		// Checking for update config
		case event := <-configEvent:
			newConfig := configToWatch{
				event.After.PubSubTriggers,
				event.After.PubSubSubscriptions,
			}
			logrus.Info("Received new config")
			if !reflect.DeepEqual(currentConfig, newConfig) {
				logrus.Info("New config found, reloading pull Server")
				// Making sure the current thread finishes before starting a new one.
				errGroup.Wait()
				// Starting a new thread with new config
				errGroup, derivedCtx, err = s.handlePulls(ctx, newConfig.PubSubTriggers)
				if err != nil {
					return err
				}
				currentConfig = newConfig
			}
		}
	}
}
