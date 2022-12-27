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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	"k8s.io/test-infra/prow/githuboauth"
	"k8s.io/test-infra/prow/plugins"
)

func handleAbort(prowJobClient prowv1.ProwJobInterface, cfg authCfgGetter, goa *githuboauth.Agent, ghc githuboauth.AuthenticatedUserIdentifier, cli deckGitHubClient, pluginAgent *plugins.ConfigAgent, log *logrus.Entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.TODO()
		name := r.URL.Query().Get("prowjob")
		l := log.WithField("prowjob", name)
		if name == "" {
			http.Error(w, "Request did not provide the 'prowjob' query parameter.", http.StatusBadRequest)
			return
		}
		pj, err := prowJobClient.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			http.Error(w, fmt.Sprintf("ProwJob not found: %v.", err), http.StatusNotFound)
			if !kerrors.IsNotFound(err) {
				// admins only care about errors other than not found
				l.WithError(err).Warning("ProwJob not found.")
			}
			return
		}
		switch r.Method {
		case http.MethodPost:
			if pj.Status.State != prowapi.TriggeredState && pj.Status.State != prowapi.PendingState {
				http.Error(w, fmt.Sprintf("Cannot abort job with state: %q.", pj.Status.State), http.StatusBadRequest)
				l.Debug("Cannot abort job with state.")
				return
			}
			// Using same permission validation as rerun, could be future work to add validation
			// unique to Abort
			allowed, user, err, code := isAllowedToRerun(r, cfg, goa, ghc, *pj, cli, pluginAgent, l)
			if err != nil {
				http.Error(w, fmt.Sprintf("Could not verify if allowed to abort: %v.", err), code)
				l.WithError(err).Debug("Could not verify if allowed to abort.")
				return
			}
			l = l.WithField("allowed", allowed)
			l.Info("Attempted abort")
			if !allowed {
				http.Error(w, "You don't have permission to abort this job.", http.StatusUnauthorized)
				l.Debug("You don't have permission to abort this job.")
				return
			}
			var abortDescription string
			if len(user) > 0 {
				abortDescription = fmt.Sprintf("%v successfully aborted %v.", user, name)
			} else {
				abortDescription = fmt.Sprintf("Successfully aborted %v.", name)
			}
			pj.Status.State = prowapi.AbortedState
			pj.Status.Description = abortDescription
			jsonPJ, err := json.Marshal(pj)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error marshal source job: %v.", err), http.StatusInternalServerError)
				l.WithError(err).Errorf("Error marshal source job.")
				return
			}
			pj, err := prowJobClient.Patch(ctx, pj.Name, ktypes.MergePatchType, jsonPJ, metav1.PatchOptions{})
			if err != nil {
				http.Error(w, fmt.Sprintf("Could not patch aborted job: %v.", err), http.StatusInternalServerError)
				l.WithError(err).Errorf("Could not patch aborted job.")
				return
			}
			l.Info(abortDescription)
			if _, err = w.Write([]byte("Job successfully aborted.")); err != nil {
				l.WithError(err).Debug(fmt.Sprintf("Error writing to abort response for %v.", pj.Name))
			}
			return
		default:
			http.Error(w, fmt.Sprintf("bad verb %v", r.Method), http.StatusMethodNotAllowed)
			return
		}
	}
}
