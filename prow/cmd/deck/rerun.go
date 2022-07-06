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
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/githuboauth"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/trigger"
)

type pluginsCfg func() *plugins.Configuration

// canTriggerJob determines whether the given user can trigger any job.
func canTriggerJob(user string, pj prowapi.ProwJob, cfg *prowapi.RerunAuthConfig, cli deckGitHubClient, pluginsCfg pluginsCfg, log *logrus.Entry) (bool, error) {
	var org string
	if pj.Spec.Refs != nil {
		org = pj.Spec.Refs.Org
	} else if len(pj.Spec.ExtraRefs) > 0 {
		org = pj.Spec.ExtraRefs[0].Org
	}

	// Then check config-level rerun auth config.
	if auth, err := cfg.IsAuthorized(org, user, cli); err != nil {
		return false, err
	} else if auth {
		return true, err
	}

	// Check job-level rerun auth config.
	if auth, err := pj.Spec.RerunAuthConfig.IsAuthorized(org, user, cli); err != nil {
		return false, err
	} else if auth {
		return true, nil
	}

	if cli == nil {
		log.Warning("No GitHub token was provided, so we cannot retrieve GitHub teams")
		return false, nil
	}

	// If the job is a presubmit and has an associated PR, and a plugin config is provided,
	// do the same checks as for /test
	if pj.Spec.Type == prowapi.PresubmitJob && pj.Spec.Refs != nil && len(pj.Spec.Refs.Pulls) > 0 {
		if pluginsCfg == nil {
			log.Info("No plugin config was provided so we cannot check if the user would be allowed to use /test.")
		} else {
			pcfg := pluginsCfg()
			pull := pj.Spec.Refs.Pulls[0]
			org := pj.Spec.Refs.Org
			repo := pj.Spec.Refs.Repo
			_, allowed, err := trigger.TrustedPullRequest(cli, pcfg.TriggerFor(org, repo), user, org, repo, pull.Number, nil)
			return allowed, err
		}
	}
	return false, nil
}

// Valid value for query parameter mode in rerun route
const (
	LATEST = "latest"
)

// handleRerun triggers a rerun of the given job if that features is enabled, it receives a
// POST request, and the user has the necessary permissions. Otherwise, it writes the config
// for a new job but does not trigger it.
func handleRerun(cfg config.Getter, prowJobClient prowv1.ProwJobInterface, createProwJob bool, acfg authCfgGetter, goa *githuboauth.Agent, ghc githuboauth.AuthenticatedUserIdentifier, cli deckGitHubClient, pluginAgent *plugins.ConfigAgent, log *logrus.Entry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("prowjob")
		l := log.WithField("prowjob", name)
		if name == "" {
			http.Error(w, "request did not provide the 'prowjob' query parameter", http.StatusBadRequest)
			return
		}
		pj, err := prowJobClient.Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			http.Error(w, fmt.Sprintf("ProwJob not found: %v", err), http.StatusNotFound)
			if !kerrors.IsNotFound(err) {
				// admins only care about errors other than not found
				l.WithError(err).Warning("ProwJob not found.")
			}
			return
		}
		newPJ := pjutil.NewProwJob(pj.Spec, pj.ObjectMeta.Labels, pj.ObjectMeta.Annotations)
		l = l.WithField("job", newPJ.Spec.Job)
		switch r.Method {
		case http.MethodGet:
			handleSerialize(w, "prowjob", newPJ, l)
		case http.MethodPost:
			if !createProwJob {
				http.Error(w, "Direct rerun feature is not enabled. Enable with the '--rerun-creates-job' flag.", http.StatusMethodNotAllowed)
				return
			}
			authConfig := acfg(newPJ.Spec.Refs, newPJ.Spec.Cluster)
			var allowed bool
			if newPJ.Spec.RerunAuthConfig.IsAllowAnyone() || authConfig.IsAllowAnyone() {
				// Skip getting the users login via GH oauth if anyone is allowed to rerun
				// jobs so that GH oauth doesn't need to be set up for private Prows.
				allowed = true
			} else {
				if goa == nil {
					msg := "GitHub oauth must be configured to rerun jobs unless 'allow_anyone: true' is specified."
					http.Error(w, msg, http.StatusInternalServerError)
					l.Error(msg)
					return
				}
				login, err := goa.GetLogin(r, ghc)
				if err != nil {
					http.Error(w, "Error retrieving GitHub login", http.StatusUnauthorized)
					l.WithError(err).Error("Error retrieving GitHub login")
					return
				}
				l = l.WithField("user", login)
				allowed, err = canTriggerJob(login, newPJ, authConfig, cli, pluginAgent.Config, l)
				if err != nil {
					http.Error(w, fmt.Sprintf("Error checking if user can trigger job: %v", err), http.StatusInternalServerError)
					l.WithError(err).Error("Error checking if user can trigger job")
					return
				}
			}

			l = l.WithField("allowed", allowed)
			l.Info("Attempted rerun")
			if !allowed {
				if _, err = w.Write([]byte("You don't have permission to rerun that job")); err != nil {
					l.WithError(err).Error("Error writing to rerun response.")
				}
				return
			}
			created, err := prowJobClient.Create(context.TODO(), &newPJ, metav1.CreateOptions{})
			if err != nil {
				l.WithError(err).Error("Error creating job")
				http.Error(w, fmt.Sprintf("Error creating job: %v", err), http.StatusInternalServerError)
				return
			}
			l = l.WithField("new-prowjob", created.Name)
			l.Info("Successfully created a rerun PJ.")
			if _, err = w.Write([]byte("Job successfully triggered. Wait 30 seconds and refresh the page for the job to show up")); err != nil {
				l.WithError(err).Error("Error writing to rerun response.")
			}
			return
		default:
			http.Error(w, fmt.Sprintf("bad verb %v", r.Method), http.StatusMethodNotAllowed)
			return
		}
	}
}
