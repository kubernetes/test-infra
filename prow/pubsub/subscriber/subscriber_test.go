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
	"fmt"
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
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/gangway"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/kube"

	v1 "k8s.io/api/core/v1"
)

var (
	trueBool  = true
	namespace = "default"
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

func (c *pubSubTestClient) subscription(id string, maxOutstandingMessages int) subscriptionInterface {
	return &fakeSubscription{name: id, messageChan: c.messageChan}
}

type fakeReporter struct {
	reported bool
}

func (r *fakeReporter) Report(_ context.Context, _ *logrus.Entry, pj *prowapi.ProwJob) ([]*prowapi.ProwJob, *reconcile.Result, error) {
	r.reported = true
	return nil, nil, nil
}

func (r *fakeReporter) ShouldReport(_ context.Context, _ *logrus.Entry, pj *prowapi.ProwJob) bool {
	return pj.Annotations[reporter.PubSubProjectLabel] != "" && pj.Annotations[reporter.PubSubTopicLabel] != ""
}

func tryGetCloneURIAndHost(pe ProwJobEvent) (cloneURI, host string) {
	refs := pe.Refs
	if refs == nil {
		return "", ""
	}
	if len(refs.Org) == 0 {
		return "", ""
	}
	if len(refs.Repo) == 0 {
		return "", ""
	}

	// If the Refs struct already has a populated CloneURI field, use that
	// instead.
	if refs.CloneURI != "" {
		if strings.HasPrefix(refs.Org, "http") {
			return refs.CloneURI, refs.Org
		}
		return refs.CloneURI, ""
	}

	org, repo := refs.Org, refs.Repo
	orgRepo := org + "/" + repo
	// Add "https://" prefix to orgRepo if this is a gerrit job.
	// (Unfortunately gerrit jobs use the full repo URL as the identifier.)
	prefix := "https://"
	if pe.Labels[kube.GerritRevision] != "" && !strings.HasPrefix(orgRepo, prefix) {
		orgRepo = prefix + orgRepo
	}
	return orgRepo, org
}

func TestProwJobEvent_ToFromMessage(t *testing.T) {
	pe := ProwJobEvent{
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
	if m.Attributes[ProwEventType] != PeriodicProwJobEvent {
		t.Errorf("%s should be %s found %s instead", ProwEventType, PeriodicProwJobEvent, m.Attributes[ProwEventType])
	}
	var newPe ProwJobEvent
	if err = newPe.FromPayload(m.Data); err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(pe, newPe) {
		t.Error("JSON encoding failed. ")
	}
}

func TestHandleMessage(t *testing.T) {
	for _, tc := range []struct {
		name, eventType string
		msg             *pubSubMessage
		pe              *ProwJobEvent
		config          *config.Config
		err             string
		labels          []string
	}{
		{
			name:      "PeriodicJobNoPubsub",
			eventType: PeriodicProwJobEvent,
			pe: &ProwJobEvent{
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
			name:      "PresubmitForGitHub",
			eventType: PresubmitProwJobEvent,
			pe: &ProwJobEvent{
				Name: "pull-github",
				Refs: &prowapi.Refs{
					Org:     "org",
					Repo:    "repo",
					BaseRef: "master",
					BaseSHA: "SHA",
					Pulls: []prowapi.Pull{
						{
							Number: 42,
						},
					},
				},
			},
			config: &config.Config{
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"org/repo": {
							{
								JobBase: config.JobBase{
									Name: "pull-github",
								},
							},
						},
					},
				},
			},
		},
		{
			name:      "PresubmitForGerrit",
			eventType: PresubmitProwJobEvent,
			pe: &ProwJobEvent{
				Name: "pull-gerrit",
				Refs: &prowapi.Refs{
					Org:     "org",
					Repo:    "repo",
					BaseRef: "master",
					BaseSHA: "SHA",
					Pulls: []prowapi.Pull{
						{
							Number: 42,
						},
					},
				},
				Labels: map[string]string{
					kube.GerritRevision: "revision",
				},
			},
			config: &config.Config{
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"https://org/repo": {
							{
								JobBase: config.JobBase{
									Name: "pull-gerrit",
								},
							},
						},
					},
				},
			},
		},
		{
			name:      "UnknownEventType",
			eventType: PeriodicProwJobEvent,
			msg: &pubSubMessage{
				Message: pubsub.Message{
					Attributes: map[string]string{
						ProwEventType: "unsupported",
					},
					Data: []byte("{}"),
				},
			},
			config: &config.Config{},
			err:    "unsupported event type: unsupported",
			labels: []string{reporter.PubSubTopicLabel, reporter.PubSubRunIDLabel, reporter.PubSubProjectLabel},
		},
		{
			name:      "NoEventType",
			eventType: PeriodicProwJobEvent,
			msg: &pubSubMessage{
				Message: pubsub.Message{
					Data: []byte("{}"),
				},
			},
			config: &config.Config{},
			err:    "unable to find \"prow.k8s.io/pubsub.EventType\" from the attributes",
			labels: []string{reporter.PubSubTopicLabel, reporter.PubSubRunIDLabel, reporter.PubSubProjectLabel},
		},
		{
			name:      "PresubmitForGerritWithInRepoConfig",
			eventType: PresubmitProwJobEvent,
			pe: &ProwJobEvent{
				Name: "pull-gerrit",
				Refs: &prowapi.Refs{
					Org:     "org",
					Repo:    "repo",
					BaseRef: "master",
					BaseSHA: "SHA",
					Pulls: []prowapi.Pull{
						{
							Number: 42,
						},
					},
				},
				Labels: map[string]string{
					kube.GerritRevision: "revision",
				},
			},
			config: &config.Config{
				JobConfig: config.JobConfig{
					ProwYAMLGetterWithDefaults: fakeProwYAMLGetter,
					ProwYAMLGetter:             fakeProwYAMLGetter,
				},
				ProwConfig: config.ProwConfig{
					PodNamespace: namespace,
					InRepoConfig: config.InRepoConfig{
						Enabled:         map[string]*bool{"*": &trueBool},
						AllowedClusters: map[string][]string{"*": {"default"}},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			fakeProwJobClient := fake.NewSimpleClientset()
			ca := &config.Agent{}
			tc.config.ProwJobNamespace = "prowjobs"
			ca.Set(tc.config)
			fr := fakeReporter{}
			gitClient, _ := (&flagutil.GitHubOptions{}).GitClientFactory("abc", nil, true, false)
			cache, _ := config.NewInRepoConfigCache(100, ca, gitClient)
			s := Subscriber{
				Metrics:            NewMetrics(),
				ProwJobClient:      fakeProwJobClient.ProwV1().ProwJobs(tc.config.ProwJobNamespace),
				ConfigAgent:        ca,
				Reporter:           &fr,
				InRepoConfigGetter: cache,
			}
			if tc.pe != nil {
				m, err := tc.pe.ToMessageOfType(tc.eventType)
				if err != nil {
					t.Error(err)
				}
				m.ID = "id"
				tc.msg = &pubSubMessage{*m}
			}
			if err := s.handleMessage(tc.msg, "", []string{"*"}); err != nil {
				if err.Error() != tc.err {
					t1.Errorf("Expected error '%v' got '%v'", tc.err, err.Error())
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

func CheckProwJob(pe *ProwJobEvent, pj *prowapi.ProwJob) error {
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
		name            string
		pe              *ProwJobEvent
		s               string
		config          *config.Config
		allowedClusters []string
		err             string
		reported        bool
		clientFails     bool
	}{
		{
			name: "PeriodicJobNoPubsub",
			pe: &ProwJobEvent{
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
			allowedClusters: []string{"*"},
		},
		{
			name: "PeriodicJobPubsubSet",
			pe: &ProwJobEvent{
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
			allowedClusters: []string{"*"},
			reported:        true,
		},
		{
			name: "ClusterNotAllowed",
			pe: &ProwJobEvent{
				Name: "test",
			},
			config: &config.Config{
				JobConfig: config.JobConfig{
					Periodics: []config.Periodic{
						{
							JobBase: config.JobBase{
								Name:    "test",
								Cluster: "precious-cluster",
							},
						},
					},
				},
			},
			allowedClusters: []string{"normal-cluster"},
			err:             "cluster precious-cluster is not allowed. Can be fixed by defining this cluster under pubsub_triggers -> allowed_clusters",
		},
		{
			name: "DefaultClusterNotAllowed",
			pe: &ProwJobEvent{
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
			allowedClusters: []string{"normal-cluster"},
			err:             "cluster  is not allowed. Can be fixed by defining this cluster under pubsub_triggers -> allowed_clusters",
		},
		{
			name: "PeriodicJobPubsubSetCreationError",
			pe: &ProwJobEvent{
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
			allowedClusters: []string{"*"},
			err:             "failed to create prowjob",
			clientFails:     true,
			reported:        true,
		},
		{
			name: "JobNotFound",
			pe: &ProwJobEvent{
				Name: "test",
			},
			config:          &config.Config{},
			allowedClusters: []string{"*"},
			err:             "failed to find associated periodic job \"test\"",
		},
		{
			name: "JobNotFoundReportNeeded",
			pe: &ProwJobEvent{
				Name: "test",
				Annotations: map[string]string{
					reporter.PubSubProjectLabel: "project",
					reporter.PubSubRunIDLabel:   "runid",
					reporter.PubSubTopicLabel:   "topic",
				},
			},
			config:          &config.Config{},
			allowedClusters: []string{"*"},
			err:             "failed to find associated periodic job \"test\"",
			reported:        true,
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
			gitClient, _ := (&flagutil.GitHubOptions{}).GitClientFactory("abc", nil, true, false)
			cache, _ := config.NewInRepoConfigCache(100, ca, gitClient)
			s := Subscriber{
				Metrics:            NewMetrics(),
				ProwJobClient:      fakeProwJobClient.ProwV1().ProwJobs(ca.Config().ProwJobNamespace),
				ConfigAgent:        ca,
				InRepoConfigGetter: cache,
				Reporter:           &fr,
			}
			l := logrus.NewEntry(logrus.New())

			cjer, err := s.peToCjer(l, tc.pe, PeriodicProwJobEvent, "fakeSubsciprtionName")
			if err != nil {
				t1.Error("programmer error: could not convert ProwJobEvent to CreateJobExecutionRequest")
			}

			_, err = gangway.HandleProwJob(l, s.getReporterFunc(l), cjer, s.ProwJobClient, s.ConfigAgent.Config(), s.InRepoConfigGetter, nil, false, tc.allowedClusters)
			if err != nil {
				if err.Error() != tc.err {
					t1.Errorf("Expected error '%v' got '%v'", tc.err, err.Error())
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
			PubSubTriggers: []config.PubSubTrigger{
				{
					Project:         "project",
					Topics:          []string{"test"},
					AllowedClusters: []string{"*"},
				},
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
				PubSubTriggers: []config.PubSubTrigger{
					{
						Project:         "project",
						Topics:          []string{"test"},
						AllowedClusters: []string{"*"},
					},
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

func TestTryGetCloneURIAndHost(t *testing.T) {
	tests := []struct {
		name             string
		pe               ProwJobEvent
		expectedCloneURI string
		expectedHost     string
	}{
		{
			name: "regular",
			pe: ProwJobEvent{
				Refs: &prowapi.Refs{
					Org:  "org",
					Repo: "repo",
				},
			},
			expectedCloneURI: "org/repo",
			expectedHost:     "org",
		},
		{
			name: "empty org",
			pe: ProwJobEvent{
				Refs: &prowapi.Refs{
					Org:  "",
					Repo: "repo",
				},
			},
			expectedCloneURI: "",
			expectedHost:     "",
		},
		{
			name: "empty repo",
			pe: ProwJobEvent{
				Refs: &prowapi.Refs{
					Org:  "org",
					Repo: "",
				},
			},
			expectedCloneURI: "",
			expectedHost:     "",
		},
		{
			name: "empty org and repo",
			pe: ProwJobEvent{
				Refs: &prowapi.Refs{
					Org:  "",
					Repo: "",
				},
			},
			expectedCloneURI: "",
			expectedHost:     "",
		},
		{
			name: "gerrit",
			pe: ProwJobEvent{
				Refs: &prowapi.Refs{
					Org:  "https://org",
					Repo: "repo",
				},
				Labels: map[string]string{
					kube.GerritRevision: "foo",
				},
			},
			expectedCloneURI: "https://org/repo",
			expectedHost:     "https://org",
		},
		{
			name: "nil Refs",
			pe: ProwJobEvent{
				Refs: nil,
			},
			expectedCloneURI: "",
			expectedHost:     "",
		},
		{
			name: "use CloneURI",
			pe: ProwJobEvent{
				Refs: &prowapi.Refs{
					Org:      "http://org",
					Repo:     "repo",
					CloneURI: "some-clone-uri",
				},
			},
			expectedCloneURI: "some-clone-uri",
			expectedHost:     "http://org",
		},
		{
			name: "use ssh-style CloneURI",
			pe: ProwJobEvent{
				Refs: &prowapi.Refs{
					Org:      "git@github.com:kubernetes/test-infra.git",
					Repo:     "repo",
					CloneURI: "some-clone-uri",
				},
			},
			expectedCloneURI: "some-clone-uri",
			expectedHost:     "",
		},
	}
	for _, tc := range tests {
		gotCloneURI, gotHost := tryGetCloneURIAndHost(tc.pe)
		if gotCloneURI != tc.expectedCloneURI {
			t.Errorf("%s: got %q, expected %q", tc.name, gotCloneURI, tc.expectedCloneURI)
		}
		if gotHost != tc.expectedHost {
			t.Errorf("%s: got %q, expected %q", tc.name, gotHost, tc.expectedHost)
		}
	}
}

func fakeProwYAMLGetter(
	c *config.Config,
	gc git.ClientFactory,
	identifier string,
	baseBranch string,
	baseSHA string,
	headSHAs ...string) (*config.ProwYAML, error) {

	presubmits := []config.Presubmit{
		{
			JobBase: config.JobBase{
				Name:      "pull-gerrit",
				Spec:      &v1.PodSpec{Containers: []v1.Container{{Name: "always-runs-inRepoConfig", Env: []v1.EnvVar{}}}},
				Namespace: &namespace,
			},
			AlwaysRun: true,
			Reporter: config.Reporter{
				Context:    "pull-gerrit",
				SkipReport: true,
			},
		},
	}
	if err := config.SetPresubmitRegexes(presubmits); err != nil {
		return nil, err
	}
	res := config.ProwYAML{
		Presubmits: presubmits,
	}
	return &res, nil
}
