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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	reporter "k8s.io/test-infra/prow/crier/reporters/pubsub"

	v1 "k8s.io/api/core/v1"
)

type pubSubTestClient struct {
	messageChan chan fakeMessage
}

type fakeSubscription struct {
	name        string
	messageChan chan fakeMessage
}

type fakeMessage pubsub.Message

func (m *fakeMessage) getAttributes() map[string]string {
	return m.Attributes
}

func (m *fakeMessage) getPayload() []byte {
	return m.Data
}

func (m *fakeMessage) getID() string {
	return m.ID
}

func (m *fakeMessage) ack()  {}
func (m *fakeMessage) nack() {}

func (s *fakeSubscription) string() string {
	return s.name
}

func (s *fakeSubscription) receive(ctx context.Context, f func(context.Context, messageInterface)) error {
	derivedCtx, cancel := context.WithCancel(ctx)
	msg := <-s.messageChan
	go func() {
		f(derivedCtx, &msg)
		cancel()
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-derivedCtx.Done():
			return fmt.Errorf("message processed")
		}
	}
}

func (c *pubSubTestClient) new(ctx context.Context, project string) (pubsubClientInterface, error) {
	return c, nil
}

func (c *pubSubTestClient) subscription(id string) subscriptionInterface {
	return &fakeSubscription{name: id, messageChan: c.messageChan}
}

type fakeReporter struct {
	reported bool
}

func (r *fakeReporter) Report(_ *logrus.Entry, pj *prowapi.ProwJob) ([]*prowapi.ProwJob, *reconcile.Result, error) {
	r.reported = true
	return nil, nil, nil
}

func (r *fakeReporter) ShouldReport(_ *logrus.Entry, pj *prowapi.ProwJob) bool {
	return pj.Annotations[reporter.PubSubProjectLabel] != "" && pj.Annotations[reporter.PubSubTopicLabel] != ""
}

func TestPeriodicProwJobEvent_ToFromMessage(t *testing.T) {
	pe := PeriodicProwJobEvent{
		Annotations: map[string]string{
			reporter.PubSubProjectLabel: "project",
			reporter.PubSubTopicLabel:   "topic",
			reporter.PubSubRunIDLabel:   "asdfasdfn",
		},
		Envs: map[string]string{
			"ENV1": "test",
			"ENV2": "test2",
		},
		Name: "ProwJobName",
	}
	m, err := pe.ToMessage()
	if err != nil {
		t.Error(err)
	}
	if m.Attributes[prowEventType] != periodicProwJobEvent {
		t.Errorf("%s should be %s found %s instead", prowEventType, periodicProwJobEvent, m.Attributes[prowEventType])
	}
	var newPe PeriodicProwJobEvent
	if err = newPe.FromPayload(m.Data); err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(pe, newPe) {
		t.Error("JSON encoding failed. ")
	}
}

func TestHandleMessage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		msg    *pubSubMessage
		pe     *PeriodicProwJobEvent
		s      string
		config *config.Config
		err    string
		labels []string
	}{
		{
			name: "PeriodicJobNoPubsub",
			pe: &PeriodicProwJobEvent{
				Name: "test",
			},
			config: &config.Config{
				JobConfig: config.JobConfig{
					Periodics: []config.Periodic{
						{
							JobBase: config.JobBase{
								Name: "test",
							},
						},
					},
				},
			},
		},
		{
			name: "UnknownEventType",
			msg: &pubSubMessage{
				Message: pubsub.Message{
					Attributes: map[string]string{
						prowEventType: "unsupported",
					},
				},
			},
			config: &config.Config{},
			err:    "unsupported event type",
			labels: []string{reporter.PubSubTopicLabel, reporter.PubSubRunIDLabel, reporter.PubSubProjectLabel},
		},
		{
			name: "NoEventType",
			msg: &pubSubMessage{
				Message: pubsub.Message{},
			},
			config: &config.Config{},
			err:    "unable to find \"prow.k8s.io/pubsub.EventType\" from the attributes",
			labels: []string{reporter.PubSubTopicLabel, reporter.PubSubRunIDLabel, reporter.PubSubProjectLabel},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			fakeProwJobClient := fake.NewSimpleClientset()
			ca := &config.Agent{}
			tc.config.ProwJobNamespace = "prowjobs"
			ca.Set(tc.config)
			s := Subscriber{
				Metrics:       NewMetrics(),
				ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs(tc.config.ProwJobNamespace),
				ConfigAgent:   ca,
			}
			if tc.pe != nil {
				m, err := tc.pe.ToMessage()
				if err != nil {
					t.Error(err)
				}
				m.ID = "id"
				tc.msg = &pubSubMessage{*m}
			}
			if err := s.handleMessage(tc.msg, tc.s); err != nil {
				if err.Error() != tc.err {
					t1.Errorf("Expected error %v got %v", tc.err, err.Error())
				} else if tc.err == "" {
					var created []*prowapi.ProwJob
					for _, action := range fakeProwJobClient.Fake.Actions() {
						switch action := action.(type) {
						case clienttesting.CreateActionImpl:
							if prowjob, ok := action.Object.(*prowapi.ProwJob); ok {
								created = append(created, prowjob)
							}
						}
					}
					if len(created) != 1 {
						t.Errorf("Expected to create 1 ProwJobs, got %d", len(created))
					}
					for _, k := range tc.labels {
						if _, ok := created[0].Labels[k]; !ok {
							t.Errorf("label %s is missing", k)
						}
					}
				}
			}
		})
	}
}

func CheckProwJob(pe *PeriodicProwJobEvent, pj *prowapi.ProwJob) error {
	// checking labels
	for label, value := range pe.Labels {
		if pj.Labels[label] != value {
			return fmt.Errorf("label %s should be set to %s, found %s instead", label, value, pj.Labels[label])
		}
	}
	// Checking Annotations
	for annotation, value := range pe.Annotations {
		if pj.Annotations[annotation] != value {
			return fmt.Errorf("annotation %s should be set to %s, found %s instead", annotation, value, pj.Annotations[annotation])
		}
	}
	if pj.Spec.PodSpec != nil {
		// Checking Envs
		for _, container := range pj.Spec.PodSpec.Containers {
			envs := map[string]string{}
			for _, env := range container.Env {
				envs[env.Name] = env.Value
			}
			for env, value := range pe.Envs {
				if envs[env] != value {
					return fmt.Errorf("env %s should be set to %s, found %s instead", env, value, pj.Annotations[env])
				}
			}
		}
	}

	return nil
}

func TestHandlePeriodicJob(t *testing.T) {
	for _, tc := range []struct {
		name        string
		pe          *PeriodicProwJobEvent
		s           string
		config      *config.Config
		err         string
		reported    bool
		clientFails bool
	}{
		{
			name: "PeriodicJobNoPubsub",
			pe: &PeriodicProwJobEvent{
				Name: "test",
			},
			config: &config.Config{
				JobConfig: config.JobConfig{
					Periodics: []config.Periodic{
						{
							JobBase: config.JobBase{
								Name: "test",
							},
						},
					},
				},
			},
		},
		{
			name: "PeriodicJobPubsubSet",
			pe: &PeriodicProwJobEvent{
				Name: "test",
				Annotations: map[string]string{
					reporter.PubSubProjectLabel: "project",
					reporter.PubSubRunIDLabel:   "runid",
					reporter.PubSubTopicLabel:   "topic",
				},
				Labels: map[string]string{
					"label1": "label1",
					"label2": "label2",
				},
				Envs: map[string]string{
					"env1": "env1",
					"env2": "env2",
				},
			},
			config: &config.Config{
				JobConfig: config.JobConfig{
					Periodics: []config.Periodic{
						{
							JobBase: config.JobBase{
								Name:        "test",
								Labels:      map[string]string{},
								Annotations: map[string]string{},
								Spec: &v1.PodSpec{
									Containers: []v1.Container{
										{
											Name: "test",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "PeriodicJobPubsubSetCreationError",
			pe: &PeriodicProwJobEvent{
				Name: "test",
				Annotations: map[string]string{
					reporter.PubSubProjectLabel: "project",
					reporter.PubSubRunIDLabel:   "runid",
					reporter.PubSubTopicLabel:   "topic",
				},
			},
			config: &config.Config{
				JobConfig: config.JobConfig{
					Periodics: []config.Periodic{
						{
							JobBase: config.JobBase{
								Name: "test",
							},
						},
					},
				},
			},
			err:         "failed to create prowjob",
			clientFails: true,
			reported:    true,
		},
		{
			name: "JobNotFound",
			pe: &PeriodicProwJobEvent{
				Name: "test",
			},
			config: &config.Config{},
			err:    "failed to find associated periodic job \"test\"",
		},
		{
			name: "JobNotFoundReportNeeded",
			pe: &PeriodicProwJobEvent{
				Name: "test",
				Annotations: map[string]string{
					reporter.PubSubProjectLabel: "project",
					reporter.PubSubRunIDLabel:   "runid",
					reporter.PubSubTopicLabel:   "topic",
				},
			},
			config:   &config.Config{},
			err:      "failed to find associated periodic job \"test\"",
			reported: true,
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			fakeProwJobClient := fake.NewSimpleClientset()
			if tc.clientFails {
				fakeProwJobClient.PrependReactor("*", "*", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, errors.New("failed to create prowjob")
				})
			}
			ca := &config.Agent{}
			tc.config.ProwJobNamespace = "prowjobs"
			ca.Set(tc.config)
			fr := fakeReporter{}
			s := Subscriber{
				Metrics:       NewMetrics(),
				ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs(ca.Config().ProwJobNamespace),
				ConfigAgent:   ca,
				Reporter:      &fr,
			}
			m, err := tc.pe.ToMessage()
			if err != nil {
				t.Error(err)
			}
			m.ID = "id"
			err = s.handlePeriodicJob(logrus.NewEntry(logrus.New()), &pubSubMessage{*m}, tc.s)
			if err != nil {
				if err.Error() != tc.err {
					t1.Errorf("Expected error %v got %v", tc.err, err.Error())
				}
			} else if tc.err == "" {
				var created []*prowapi.ProwJob
				for _, action := range fakeProwJobClient.Fake.Actions() {
					switch action := action.(type) {
					case clienttesting.CreateActionImpl:
						if prowjob, ok := action.Object.(*prowapi.ProwJob); ok {
							created = append(created, prowjob)
							if err := CheckProwJob(tc.pe, prowjob); err != nil {
								t.Error(err)
							}
						}
					}
				}
				if len(created) != 1 {
					t.Errorf("Expected to create 1 ProwJobs, got %d", len(created))
				}
			}

			if fr.reported != tc.reported {
				t1.Errorf("Expected Reporting: %t, found: %t", tc.reported, fr.reported)
			}
		})
	}
}

func TestPushServer_ServeHTTP(t *testing.T) {
	for _, tc := range []struct {
		name         string
		url          string
		secret       string
		pushRequest  interface{}
		pe           *PeriodicProwJobEvent
		expectedCode int
	}{
		{
			name:   "WrongToken",
			secret: "wrongToken",
			url:    "https://prow.k8s.io/push",
			pushRequest: pushRequest{
				Message: message{
					ID: "runid",
				},
			},
			expectedCode: http.StatusForbidden,
		},
		{
			name: "NoToken",
			url:  "https://prow.k8s.io/push",
			pushRequest: pushRequest{
				Message: message{
					ID: "runid",
				},
			},
			expectedCode: http.StatusNotModified,
		},
		{
			name:   "RightToken",
			secret: "secret",
			url:    "https://prow.k8s.io/push?token=secret",
			pushRequest: pushRequest{
				Message: message{
					ID: "runid",
				},
			},
			expectedCode: http.StatusNotModified,
		},
		{
			name:         "InvalidPushRequest",
			secret:       "secret",
			url:          "https://prow.k8s.io/push?token=secret",
			pushRequest:  "invalid",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:        "SuccessToken",
			secret:      "secret",
			url:         "https://prow.k8s.io/push?token=secret",
			pushRequest: pushRequest{},
			pe: &PeriodicProwJobEvent{
				Name: "test",
			},
			expectedCode: http.StatusOK,
		},
		{
			name:        "SuccessNoToken",
			url:         "https://prow.k8s.io/push",
			pushRequest: pushRequest{},
			pe: &PeriodicProwJobEvent{
				Name: "test",
			},
			expectedCode: http.StatusOK,
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			c := &config.Config{
				ProwConfig: config.ProwConfig{
					ProwJobNamespace: "prowjobs",
				},
				JobConfig: config.JobConfig{
					Periodics: []config.Periodic{
						{
							JobBase: config.JobBase{
								Name: "test",
							},
						},
					},
				},
			}
			fakeProwJobClient := fake.NewSimpleClientset()
			pushServer := PushServer{
				Subscriber: &Subscriber{
					ConfigAgent:   &config.Agent{},
					Metrics:       NewMetrics(),
					ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs(c.ProwJobNamespace),
					Reporter:      &fakeReporter{},
				},
			}
			pushServer.Subscriber.ConfigAgent.Set(c)
			pushServer.TokenGenerator = func() []byte { return []byte(tc.secret) }

			body := new(bytes.Buffer)

			if tc.pe != nil {
				msg, err := tc.pe.ToMessage()
				if err != nil {
					t.Error(err)
				}
				tc.pushRequest = pushRequest{
					Message: message{
						Attributes: msg.Attributes,
						ID:         "id",
						Data:       msg.Data,
					},
				}
			}

			if err := json.NewEncoder(body).Encode(tc.pushRequest); err != nil {
				t1.Errorf(err.Error())
			}
			req := httptest.NewRequest(http.MethodPost, tc.url, body)
			w := httptest.NewRecorder()
			pushServer.ServeHTTP(w, req)
			resp := w.Result()
			if resp.StatusCode != tc.expectedCode {
				t1.Errorf("exected code %d got %d", tc.expectedCode, resp.StatusCode)
			}
		})
	}
}

func TestPullServer_RunShutdown(t *testing.T) {
	s := &Subscriber{
		ConfigAgent:   &config.Agent{},
		ProwJobClient: fake.NewSimpleClientset().ProwV1().ProwJobs("prowjobs"),
		Metrics:       NewMetrics(),
	}
	c := &config.Config{}
	s.ConfigAgent.Set(c)
	pullServer := PullServer{
		Subscriber: s,
		Client:     &pubSubTestClient{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	errChan := make(chan error)
	go func() {
		errChan <- pullServer.Run(ctx)
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	err := <-errChan
	if err != nil {
		if !strings.HasPrefix(err.Error(), "context canceled") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestPullServer_RunHandlePullFail(t *testing.T) {
	s := &Subscriber{
		ConfigAgent:   &config.Agent{},
		ProwJobClient: fake.NewSimpleClientset().ProwV1().ProwJobs("prowjobs"),
		Metrics:       NewMetrics(),
	}
	c := &config.Config{
		ProwConfig: config.ProwConfig{
			PubSubSubscriptions: map[string][]string{
				"project": {"test"},
			},
		},
	}
	messageChan := make(chan fakeMessage, 1)
	s.ConfigAgent.Set(c)
	pullServer := PullServer{
		Subscriber: s,
		Client:     &pubSubTestClient{messageChan: messageChan},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	errChan := make(chan error)
	messageChan <- fakeMessage{
		Attributes: map[string]string{},
		ID:         "test",
	}
	defer cancel()
	go func() {
		errChan <- pullServer.Run(ctx)
	}()
	err := <-errChan
	// Should fail since Pub/Sub cred are not set
	if !strings.HasPrefix(err.Error(), "message processed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPullServer_RunConfigChange(t *testing.T) {
	s := &Subscriber{
		ConfigAgent:   &config.Agent{},
		ProwJobClient: fake.NewSimpleClientset().ProwV1().ProwJobs("prowjobs"),
		Metrics:       NewMetrics(),
	}
	c := &config.Config{}
	messageChan := make(chan fakeMessage, 1)
	s.ConfigAgent.Set(c)
	pullServer := PullServer{
		Subscriber: s,
		Client:     &pubSubTestClient{messageChan: messageChan},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	errChan := make(chan error)
	go func() {
		errChan <- pullServer.Run(ctx)
	}()
	select {
	case <-errChan:
		t.Error("should not fail")
	case <-time.After(10 * time.Millisecond):
		newConfig := &config.Config{
			ProwConfig: config.ProwConfig{
				PubSubSubscriptions: map[string][]string{
					"project": {"test"},
				},
			},
		}
		s.ConfigAgent.Set(newConfig)
		messageChan <- fakeMessage{
			Attributes: map[string]string{},
			ID:         "test",
		}
		err := <-errChan
		if !strings.HasPrefix(err.Error(), "message processed") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}
