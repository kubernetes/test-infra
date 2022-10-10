/*
Copyright 2017 The Kubernetes Authors.

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

// Package externalplugins provides the plugin help components to be compiled into external plugin binaries.
// Since external plugins only need to serve a "/help" endpoint this package just provides an
// http.HandlerFunc that wraps an ExternalPluginHelpProvider function.
package externalplugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/pluginhelp"
)

// ExternalPluginHelpProvider is a func type that returns a PluginHelp struct for an external
// plugin based on the specified enabledRepos.
type ExternalPluginHelpProvider func([]config.OrgRepo) (*pluginhelp.PluginHelp, error)

// ServeExternalPluginHelp returns a HandlerFunc that serves plugin help information that is
// provided by the specified ExternalPluginHelpProvider.
func ServeExternalPluginHelp(mux *http.ServeMux, log *logrus.Entry, provider ExternalPluginHelpProvider) {
	mux.HandleFunc(
		"/help",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")

			serverError := func(action string, err error) {
				log.WithError(err).Errorf("Error %s.", action)
				msg := fmt.Sprintf("500 Internal server error %s: %v", action, err)
				http.Error(w, msg, http.StatusInternalServerError)
			}

			if r.Method != http.MethodPost {
				log.Errorf("Invalid request method: %v.", r.Method)
				http.Error(w, "405 Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			b, err := io.ReadAll(r.Body)
			if err != nil {
				serverError("reading request body", err)
				return
			}
			var enabledRepos []config.OrgRepo
			if err := json.Unmarshal(b, &enabledRepos); err != nil {
				serverError("unmarshaling request body", err)
				return
			}
			if provider == nil {
				serverError("generating plugin help", errors.New("help provider is nil"))
				return
			}
			help, err := provider(enabledRepos)
			if err != nil {
				serverError("generating plugin help", err)
				return
			}
			b, err = json.Marshal(help)
			if err != nil {
				serverError("marshaling plugin help", err)
				return
			}

			fmt.Fprint(w, string(b))
		},
	)
}
