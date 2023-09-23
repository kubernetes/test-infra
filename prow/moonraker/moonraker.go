/*
Copyright 2023 The Kubernetes Authors.

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

package moonraker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

const (
	PathGetInrepoconfig = "inrepoconfig"
	PathPing            = "ping"
)

type Moonraker struct {
	ConfigAgent       *config.Agent
	InRepoConfigCache *config.InRepoConfigCache
}

type configSectionsToWatch struct {
	config.InRepoConfig // Map of allowlisted inrepoconfig URLs to clone from
}

// payload is the message payload we use for Moonraker. For
// forward-compatibility, we don't use a prowapi.Refs directly.
type payload struct {
	Refs prowapi.Refs `json:"refs"`
}

type ProwYAMLGetter interface {
	GetProwYAML(payload *payload) (*config.ProwYAML, error)
}

// ServePing responds with "pong". It's meant to be used by clients to check if
// the service is up.
func (mr *Moonraker) ServePing(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong")
}

// serveGetInrepoconfig returns a ProwYAML object marshaled into JSON.
func (mr *Moonraker) ServeGetInrepoconfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logrus.WithError(err).Info("unable to read request")
		http.Error(w, fmt.Sprintf("bad request %v", err), http.StatusBadRequest)
		return
	}

	payload := &payload{}
	err = json.Unmarshal(body, payload)
	if err != nil {
		logrus.WithError(err).Info("unable to unmarshal getInrepoconfig request")
		http.Error(w, fmt.Sprintf("unable to unmarshal getInrepoconfig request: %v", err), http.StatusBadRequest)
		return
	}

	baseSHAGetter := func() (string, error) {
		return payload.Refs.BaseSHA, nil
	}
	var headSHAGetters []func() (string, error)
	for _, pull := range payload.Refs.Pulls {
		pull := pull
		headSHAGetters = append(headSHAGetters, func() (string, error) {
			return pull.SHA, nil
		})
	}
	identifier := payload.Refs.Org + "/" + payload.Refs.Repo

	prowYAML, err := mr.InRepoConfigCache.GetProwYAMLWithoutDefaults(identifier, baseSHAGetter, headSHAGetters...)
	if err != nil {
		logrus.WithError(err).Error("unable to retrieve inrepoconfig ProwYAML")
		http.Error(w, fmt.Sprintf("unable to retrieve inrepoconfig ProwYAML: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(prowYAML); err != nil {
		logrus.WithError(err).Error("unable to encode inrepoconfig ProwYAML into JSON")
		http.Error(w, fmt.Sprintf("unable to encode inrepoconfig ProwYAML into JSON: %v", err), http.StatusBadRequest)
		return
	}
}

func (mr *Moonraker) RunConfigWatcher(ctx context.Context) error {
	configEvent := make(chan config.Delta, 2)
	mr.ConfigAgent.Subscribe(configEvent)

	var err error
	defer func() {
		if err != nil {
			logrus.WithError(ctx.Err()).Error("ConfigWatcher shutting down.")
		}
		logrus.Debug("Pull server shutting down.")
	}()
	currentConfig := configSectionsToWatch{
		mr.ConfigAgent.Config().InRepoConfig,
	}

	for {
		select {
		// Parent context. Shutdown
		case <-ctx.Done():
			return nil
		// Checking for update config
		case event := <-configEvent:
			newConfig := configSectionsToWatch{
				event.After.InRepoConfig,
			}
			logrus.Info("Received new config")
			if !reflect.DeepEqual(currentConfig, newConfig) {
				logrus.Info("New config found, resetting Config in ConfigAgent")
				mr.ConfigAgent.SetWithoutBroadcast(&event.After)
				currentConfig = newConfig
			}
		}
	}
}
