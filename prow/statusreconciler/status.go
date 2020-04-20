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

package statusreconciler

import (
	"context"
	"io/ioutil"
	"time"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io"
)

type storedState struct {
	config.Config
}

type statusClient interface {
	Load() (chan config.Delta, error)
	Save() error
}

// opener has methods to read and write paths
type opener interface {
	Reader(ctx context.Context, path string) (io.ReadCloser, error)
	Writer(ctx context.Context, path string, opts ...io.WriterOptions) (io.WriteCloser, error)
}

type statusController struct {
	logger        *logrus.Entry
	opener        opener
	statusURI     string
	configPath    string
	jobConfigPath string

	storedState
	config.Agent
}

func (s *statusController) Load() (chan config.Delta, error) {
	s.Agent = config.Agent{}
	state, err := s.loadState()
	if err == nil {
		s.Agent.Set(&state.Config)
	}
	changes := make(chan config.Delta)
	s.Agent.Subscribe(changes)

	if err := s.Agent.Start(s.configPath, s.jobConfigPath); err != nil {
		s.logger.WithError(err).Fatal("Error starting config agent.")
		return nil, err
	}
	return changes, nil
}

func (s *statusController) Save() error {
	if s.statusURI == "" {
		return nil
	}
	entry := s.logger.WithField("path", s.statusURI)
	current := s.Agent.Config()
	buf, err := yaml.Marshal(current)
	if err != nil {
		entry.WithError(err).Warn("Cannot marshal state")
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	writer, err := s.opener.Writer(ctx, s.statusURI)
	if err != nil {
		entry.WithError(err).Warn("Cannot open state writer")
		return err
	}
	if _, err = writer.Write(buf); err != nil {
		entry.WithError(err).Warn("Cannot write state")
		io.LogClose(writer)
		return err
	}
	if err := writer.Close(); err != nil {
		entry.WithError(err).Warn("Failed to close written state")
	}
	entry.Debug("Saved status state")
	return nil
}

func (s *statusController) loadState() (storedState, error) {
	var state storedState
	if s.statusURI == "" {
		s.logger.Debug("No stored state configured")
		return state, nil
	}
	entry := s.logger.WithField("path", s.statusURI)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reader, err := s.opener.Reader(ctx, s.statusURI)
	if err != nil {
		entry.WithError(err).Warn("Cannot open stored state")
		return state, err
	}
	defer io.LogClose(reader)

	buf, err := ioutil.ReadAll(reader)
	if err != nil {
		entry.WithError(err).Warn("Cannot read stored state")
		return state, err
	}

	if err := yaml.Unmarshal(buf, &state); err != nil {
		entry.WithError(err).Warn("Cannot unmarshal stored state")
		return state, err
	}
	return state, nil
}

func (s *statusController) Config() *config.Config {
	return s.Agent.Config()
}
