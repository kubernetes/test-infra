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
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/shurcooL/githubql"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

const (
	githubEndpoint = "https://api.github.com"
	tokenSession   = "access-token-session"
	tokenKey       = "access-token"
)

type githubClient interface {
	Query(context.Context, interface{}, map[string]interface{}) error
}

// PullRequestQueryHandler defines an interface that query handlers should implement.
type PullRequestQueryHandler interface {
	Query(context.Context, githubClient) ([]PullRequest, error)
}

// UserData represents data returned to client request to the endpoint. It has a flag that indicates
// whether the user has logged in his github or not and list of open pull requests owned by the
// user.
type UserData struct {
	Login        bool
	PullRequests []PullRequest
}

// Dashboard Agent is responsible for handling request to /user-dashboard endpoint. It will serve
// list of open pull requests owned by the user.
type DashboardAgent struct {
	goac *config.GithubOAuthConfig

	log *logrus.Entry
}

type Label struct {
	ID   githubql.ID
	Name githubql.String
}

type PullRequest struct {
	Number githubql.Int
	Title  githubql.String
	Author struct {
		Login githubql.String
	}
	BaseRef struct {
		Name   githubql.String
		Prefix githubql.String
	}
	HeadRefOID githubql.String `graphql:"headRefOid"`
	Repository struct {
		Name          githubql.String
		NameWithOwner githubql.String
		Owner         struct {
			Login githubql.String
		}
	}
	Labels struct {
		Nodes []struct {
			Label Label `graphql:"... on Label"`
		}
	} `graphql:"labels(first: 100)"`
	Milestone struct {
		ID     githubql.ID
		Closed githubql.Boolean
	}
}

type PullRequestQuery struct {
	Viewer struct {
		PullRequests struct {
			PageInfo struct {
				HasNextPage githubql.Boolean
				EndCursor   githubql.String
			}
			Nodes []struct {
				PullRequest PullRequest `graphql:"... on PullRequest"`
			}
		} `graphql:"pullRequests(first: 100, after: $prsCursor, states: [OPEN])"`
	}
}

// Returns new user dashboard agent.
func NewDashboardAgent(config *config.GithubOAuthConfig, log *logrus.Entry) *DashboardAgent {
	return &DashboardAgent{
		goac: config,
		log:  log,
	}
}

// HandleUserDashboard returns a http handler function that handles request to /user-dashboard
// endpoint. The handler takes user access token stored in the cookie to query to Github on behalf
// of the user and serve the data in return. The Query handler is passed to the method so as it
// can be mocked in the unit test..
func (da *DashboardAgent) HandleUserDashboard(queryHandler PullRequestQueryHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serverError := func(action string, err error) {
			da.log.WithError(err).Errorf("Error %s.", action)
			msg := fmt.Sprintf("500 Internal server error %s: %v", action, err)
			http.Error(w, msg, http.StatusInternalServerError)
		}

		session, err := da.goac.CookieStore.Get(r, tokenSession)
		if err != nil {
			serverError("Error with getting git token session.", err)
			return
		}
		token, ok := session.Values[tokenKey].(*oauth2.Token)
		data := UserData{
			Login: false,
		}
		if ok && token.Valid() {
			data.Login = true
			ghc := github.NewClient(token.AccessToken, githubEndpoint)
			pullRequests, err := queryHandler.Query(context.Background(), ghc)
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
}

// Query function that returns a list of open pull requests owned by the user whose access token
// is consumed by the github client.
func (da *DashboardAgent) Query(ctx context.Context, ghc githubClient) ([]PullRequest, error) {
	var prs = []PullRequest{}
	vars := map[string]interface{}{
		"prsCursor": (*githubql.String)(nil),
	}
	for {
		pq := PullRequestQuery{}
		if err := ghc.Query(ctx, &pq, vars); err != nil {
			return nil, err
		}
		for _, n := range pq.Viewer.PullRequests.Nodes {
			prs = append(prs, n.PullRequest)
		}
		if !pq.Viewer.PullRequests.PageInfo.HasNextPage {
			break
		}
		vars["prsCursor"] = githubql.NewString(pq.Viewer.PullRequests.PageInfo.EndCursor)
	}

	return prs, nil
}
