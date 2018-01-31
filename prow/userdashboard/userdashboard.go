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

package userdashboard

import (
	"github.com/sirupsen/logrus"
	"net/http"
	"k8s.io/test-infra/prow/config"
	"golang.org/x/oauth2"
	"context"
	"k8s.io/test-infra/prow/github"
	"github.com/shurcooL/githubql"
	"encoding/json"
	"fmt"
)

const githubEndpoint = "https://api.github.com"


type githubClient interface {
	Query(context.Context, interface{}, map[string]interface{}) error
}

type UserData struct {
	Login bool
	PullRequests []PullRequest
}

type DashboardAgent struct {
	gitConfig *config.GitOAuthConfig

	log *logrus.Entry
}

type Label struct {
	Id githubql.ID
	name githubql.String
}

type PullRequest struct{
	Number githubql.Int
	Author struct {
		Login githubql.String
	}
	Repository struct {
		Name          githubql.String
		NameWithOwner githubql.String
		Owner         struct {
			Login githubql.String
		}
	}
	Labels struct {
		Nodes []struct {
			Label Label
		}
	} `graphql:"labels(first: 100)"`
}

type PullRequestQuery struct {
	User struct {
		PullRequests struct {
			PageInfo struct {
				HasNextPage githubql.Boolean
				EndCursor githubql.String
			}
			Nodes []struct {
				PullRequest PullRequest
			}
		} `graphql:"pullRequests(first: 100, after: $prsCursor, state: $prsStates)"`
	}
}

func NewDashboardAgent(config *config.GitOAuthConfig, log *logrus.Entry) *DashboardAgent {
	return &DashboardAgent{
		gitConfig: config,
		log:  log,
	}
}

func (da *DashboardAgent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serverError := func(action string, err error) {
		da.log.WithError(err).Errorf("Error %s.", action)
		msg := fmt.Sprintf("500 Internal server error %s: %v", action, err)
		http.Error(w, msg, http.StatusInternalServerError)
	}

	session, err := da.gitConfig.CookieStore.Get(r, da.gitConfig.GitTokenSession)
	if err != nil {
		serverError("Error with getting git token session.", err)
		return
	}
	token, ok := session.Values[da.gitConfig.GitTokenKey].(*oauth2.Token)
	data := UserData{}

	if !ok || !token.Valid() {
		data.Login = false
	} else {
		data.Login = true
		ghc := github.NewClient(token.AccessToken, githubEndpoint)
		pullRequests, err := search(context.Background(), ghc)
		if err != nil {
			serverError("Error with querying user data.", err)
			return
		} else {
			data.PullRequests = pullRequests
		}
	}

	marshaledData, err := json.Marshal(data)
	if err != nil {
		da.log.WithError(err).Error("Error with marshalling user data.")
	}

	w.Write(marshaledData)
}

func search(ctx context.Context, ghc githubClient) ([]PullRequest, error){
	var prs = []PullRequest{}
	vars := map[string]interface{}{
		"$prsStates":         githubql.PullRequestStateOpen,
		"$prsCursor": 				(*githubql.String)(nil),
	}
	for {
		pq := PullRequestQuery{}
		if err := ghc.Query(ctx, &pq, vars); err != nil {
			return nil, err
		}
		for _, n := range pq.User.PullRequests.Nodes {
			prs = append(prs, n.PullRequest)
		}
		if !pq.User.PullRequests.PageInfo.HasNextPage {
			break
		}
		vars["prsCursor"] = githubql.NewString(pq.User.PullRequests.PageInfo.EndCursor)
	}

	return prs, nil
}




