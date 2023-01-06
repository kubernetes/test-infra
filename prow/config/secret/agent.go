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

// Package secret implements an agent to read and reload the secrets.
package secret

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/secretutil"
)

// secretAgent is the singleton that loads secrets for us
var secretAgent *agent

func init() {
	secretAgent = &agent{
		secretsMap:        map[string]secretReloader{},
		ReloadingCensorer: secretutil.NewCensorer(),
	}
	logrus.SetFormatter(logrusutil.NewFormatterWithCensor(logrus.StandardLogger().Formatter, secretAgent.ReloadingCensorer))
}

// Start creates goroutines to monitor the files that contain the secret value.
// Additionally, Start wraps the current standard logger formatter with a
// censoring formatter that removes secret occurrences from the logs.
func (a *agent) Start(paths []string) error {
	a.secretsMap = make(map[string]secretReloader, len(paths))
	a.ReloadingCensorer = secretutil.NewCensorer()

	for _, path := range paths {
		if err := a.Add(path); err != nil {
			return fmt.Errorf("failed to load secret at %s: %w", path, err)
		}
	}

	logrus.SetFormatter(logrusutil.NewFormatterWithCensor(logrus.StandardLogger().Formatter, a.ReloadingCensorer))

	return nil
}

// Add registers a new path to the agent.
func Add(paths ...string) error {
	for _, path := range paths {
		if err := secretAgent.Add(path); err != nil {
			return err
		}
	}
	return nil
}

// AddWithParser registers a new path to the agent. The secret will only be updated if it can
// be successfully parsed. The returned getter must be kept, as it is the only way of accessing
// the typed secret.
func AddWithParser[T any](path string, parsingFN func([]byte) (T, error)) (func() T, error) {
	loader := &parsingSecretReloader[T]{
		path:      path,
		parsingFN: parsingFN,
	}
	return loader.get, secretAgent.add(path, loader)
}

// GetSecret returns the value of a secret stored in a map.
func GetSecret(secretPath string) []byte {
	return secretAgent.GetSecret(secretPath)
}

// GetTokenGenerator returns a function that gets the value of a given secret.
func GetTokenGenerator(secretPath string) func() []byte {
	return func() []byte {
		return GetSecret(secretPath)
	}
}

func Censor(content []byte) []byte {
	return secretAgent.Censor(content)
}

// agent watches a path and automatically loads the secrets stored.
type agent struct {
	sync.RWMutex
	secretsMap map[string]secretReloader
	*secretutil.ReloadingCensorer
}

type secretReloader interface {
	getRaw() []byte
	start(reloadCensor func()) error
}

// Add registers a new path to the agent.
func (a *agent) Add(path string) error {
	return a.add(path, &parsingSecretReloader[[]byte]{
		path:      path,
		parsingFN: func(b []byte) ([]byte, error) { return b, nil },
	})
}

func (a *agent) add(path string, loader secretReloader) error {
	if err := loader.start(a.refreshCensorer); err != nil {
		return err
	}

	a.setSecret(path, loader)

	return nil
}

// GetSecret returns the value of a secret stored in a map.
func (a *agent) GetSecret(secretPath string) []byte {
	a.RLock()
	defer a.RUnlock()
	if val, set := a.secretsMap[secretPath]; set {
		return val.getRaw()
	}
	return nil
}

// setSecret sets a value in a map of secrets.
func (a *agent) setSecret(secretPath string, secretValue secretReloader) {
	a.Lock()
	a.secretsMap[secretPath] = secretValue
	a.Unlock()
	a.refreshCensorer()
}

// refreshCensorer should be called when the secrets map changes
func (a *agent) refreshCensorer() {
	var secrets [][]byte
	a.RLock()
	for _, value := range a.secretsMap {
		secrets = append(secrets, value.getRaw())
	}
	a.RUnlock()
	a.ReloadingCensorer.RefreshBytes(secrets...)
}

// GetTokenGenerator returns a function that gets the value of a given secret.
func (a *agent) GetTokenGenerator(secretPath string) func() []byte {
	return func() []byte {
		return a.GetSecret(secretPath)
	}
}

// Censor replaces sensitive parts of the content with a placeholder.
func (a *agent) Censor(content []byte) []byte {
	a.RLock()
	defer a.RUnlock()
	if a.ReloadingCensorer == nil {
		// there's no constructor for an agent so we can't ensure that everyone is
		// trying to censor *after* actually loading a secret ...
		return content
	}
	return secretutil.AdaptCensorer(a.ReloadingCensorer)(content)
}

func (a *agent) getSecrets() sets.String {
	a.RLock()
	defer a.RUnlock()
	secrets := sets.NewString()
	for _, v := range a.secretsMap {
		secrets.Insert(string(v.getRaw()))
	}
	return secrets
}
