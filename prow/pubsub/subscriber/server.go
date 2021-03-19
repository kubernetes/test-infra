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
	"net/http"
	"reflect"
	"strconv"

	"github.com/sirupsen/logrus"

	"cloud.google.com/go/pubsub"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
	"k8s.io/test-infra/prow/config"
)

const (
	tokenLabel = "token"
)

type message struct {
	Attributes map[string]string
	Data       []byte
	ID         string `json:"message_id"`
}

// pushRequest is the format of the push Pub/Sub subscription received form the WebHook.
type pushRequest struct {
	Message      message
	Subscription string
}

// PushServer implements http.Handler. It validates incoming Pub/Sub subscriptions handle them.
type PushServer struct {
	Subscriber     *Subscriber
	TokenGenerator func() []byte
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

// ServeHTTP validates an incoming Push Pub/Sub subscription and handle them.
func (s *PushServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	HTTPCode := http.StatusOK
	subscription := "unknown-subscription"
	var finalError error

	defer func() {
		s.Subscriber.Metrics.ResponseCounter.With(prometheus.Labels{
			subscriptionLabel: subscription,
			responseCodeLabel: strconv.Itoa(HTTPCode),
		}).Inc()
		if finalError != nil {
			http.Error(w, finalError.Error(), HTTPCode)
		}
	}()

	if s.TokenGenerator != nil {
		token := r.URL.Query().Get(tokenLabel)
		if token != string(s.TokenGenerator()) {
			finalError = fmt.Errorf("wrong token")
			HTTPCode = http.StatusForbidden
			return
		}
	}
	// Get the payload and act on it.
	pr := &pushRequest{}
	if err := json.NewDecoder(r.Body).Decode(pr); err != nil {
		finalError = err
		HTTPCode = http.StatusBadRequest
		return
	}

	msg := pubsub.Message{
		Data:       pr.Message.Data,
		ID:         pr.Message.ID,
		Attributes: pr.Message.Attributes,
	}

	if err := s.Subscriber.handleMessage(&pubSubMessage{Message: msg}, pr.Subscription); err != nil {
		finalError = err
		HTTPCode = http.StatusNotModified
		return
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
	subscription(id string) subscriptionInterface
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

// Subscription creates a subscription from the Cloud Pub/Sub Client
func (c *pubSubClient) subscription(id string) subscriptionInterface {
	return &pubSubSubscription{
		sub: c.client.Subscription(id),
	}
}

// handlePulls pull for Pub/Sub subscriptions and handle them.
func (s *PullServer) handlePulls(ctx context.Context, projectSubscriptions config.PubsubSubscriptions) (*errgroup.Group, context.Context, error) {
	// Since config might change we need be able to cancel the current run
	errGroup, derivedCtx := errgroup.WithContext(ctx)
	for project, subscriptions := range projectSubscriptions {
		client, err := s.Client.new(ctx, project)
		if err != nil {
			return errGroup, derivedCtx, err
		}
		for _, subName := range subscriptions {
			sub := client.subscription(subName)
			errGroup.Go(func() error {
				logrus.Infof("Listening for subscription %s on project %s", sub.string(), project)
				defer logrus.Warnf("Stopped Listening for subscription %s on project %s", sub.string(), project)
				err := sub.receive(derivedCtx, func(ctx context.Context, msg messageInterface) {
					if err = s.Subscriber.handleMessage(msg, sub.string()); err != nil {
						s.Subscriber.Metrics.ACKMessageCounter.With(prometheus.Labels{subscriptionLabel: sub.string()}).Inc()
					} else {
						s.Subscriber.Metrics.NACKMessageCounter.With(prometheus.Labels{subscriptionLabel: sub.string()}).Inc()
					}
					msg.ack()
				})
				if err != nil {
					logrus.WithError(err).Errorf("failed to listen for subscription %s on project %s", sub.string(), project)
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
			logrus.WithError(ctx.Err()).Error("Pull server shutting down")
		}
		logrus.Warn("Pull server shutting down")
	}()
	currentConfig := s.Subscriber.ConfigAgent.Config().PubSubSubscriptions
	errGroup, derivedCtx, err := s.handlePulls(ctx, currentConfig)
	if err != nil {
		return err
	}

	for {
		select {
		// Parent context. Shutdown
		case <-ctx.Done():
			return ctx.Err()
		// Current thread context, it may be failing already
		case <-derivedCtx.Done():
			err = errGroup.Wait()
			return err
		// Checking for update config
		case event := <-configEvent:
			newConfig := event.After.PubSubSubscriptions
			logrus.Info("Received new config")
			if !reflect.DeepEqual(currentConfig, newConfig) {
				logrus.Warn("New config found, reloading pull Server")
				// Making sure the current thread finishes before starting a new one.
				errGroup.Wait()
				// Starting a new thread with new config
				errGroup, derivedCtx, err = s.handlePulls(ctx, newConfig)
				if err != nil {
					return err
				}
				currentConfig = newConfig
			}
		}
	}
}
