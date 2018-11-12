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

package prstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	gogithub "github.com/google/go-github/github"
	"github.com/gorilla/sessions"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"k8s.io/test-infra/pkg/ghclient"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

const (
	loginSession   = "github_login"
	githubEndpoint = "https://api.github.com"
	tokenSession   = "access-token-session"
	tokenKey       = "access-token"
	loginKey       = "login"
)

type githubClient interface {
	Query(context.Context, interface{}, map[string]interface{}) error
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
}

// PullRequestQueryHandler defines an interface that query handlers should implement.
type PullRequestQueryHandler interface {
	HeadContexts(ghc githubClient, pr PullRequest) ([]Context, error)
	QueryPullRequests(context.Context, githubClient, string) ([]PullRequest, error)
	GetUser(*ghclient.Client) (*gogithub.User, error)
}

// UserData represents data returned to client request to the endpoint. It has a flag that indicates
// whether the user has logged in his github or not and list of open pull requests owned by the
// user.
type UserData struct {
	Login                    bool
	PullRequestsWithContexts []PullRequestWithContext
}

// PullRequestWithContext contains a pull request with its latest context.
type PullRequestWithContext struct {
	Contexts    []Context
	PullRequest PullRequest
}

// Dashboard Agent is responsible for handling request to /pr-status endpoint. It will serve
// list of open pull requests owned by the user.
type DashboardAgent struct {
	repos []string
	goac  *config.GithubOAuthConfig

	log *logrus.Entry
}

type Label struct {
	ID   githubql.ID
	Name githubql.String
}

// Context represent github contexts.
type Context struct {
	Context     string
	Description string
	State       string
}

// PullRequest holds graphql response data for github pull request.
type PullRequest struct {
	Number githubql.Int
	Merged githubql.Boolean
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
		Title githubql.String
	}
	Mergeable githubql.MergeableState
}

type UserLoginQuery struct {
	Viewer struct {
		Login githubql.String
	}
}

type searchQuery struct {
	RateLimit struct {
		Cost      githubql.Int
		Remaining githubql.Int
	}
	Search struct {
		PageInfo struct {
			HasNextPage githubql.Boolean
			EndCursor   githubql.String
		}
		Nodes []struct {
			PullRequest PullRequest `graphql:"... on PullRequest"`
		}
	} `graphql:"search(type: ISSUE, first: 100, after: $searchCursor, query: $query)"`
}

// Returns new user dashboard agent.
func NewDashboardAgent(repos []string, config *config.GithubOAuthConfig, log *logrus.Entry) *DashboardAgent {
	return &DashboardAgent{
		repos: repos,
		goac:  config,
		log:   log,
	}
}

func invalidateGitHubSession(w http.ResponseWriter, r *http.Request, session *sessions.Session) error {
	// Invalidate github login session
	http.SetCookie(w, &http.Cookie{
		Name:    loginSession,
		Path:    "/",
		Expires: time.Now().Add(-time.Hour * 24),
		MaxAge:  -1,
		Secure:  true,
	})

	// Invalidate access token session
	session.Options.MaxAge = -1
	return session.Save(r, w)
}

// HandlePrStatus returns a http handler function that handles request to /pr-status
// endpoint. The handler takes user access token stored in the cookie to query to Github on behalf
// of the user and serve the data in return. The Query handler is passed to the method so as it
// can be mocked in the unit test..
func (da *DashboardAgent) HandlePrStatus(queryHandler PullRequestQueryHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serverError := func(action string, err error) {
			da.log.WithError(err).Errorf("Error %s.", action)
			msg := fmt.Sprintf("500 Internal server error %s: %v", action, err)
			http.Error(w, msg, http.StatusInternalServerError)
		}

		data := UserData{
			Login: false,
		}

		// Get existing session. Invalidate everything if we fail and continue as
		// if not logged in.
		session, err := da.goac.CookieStore.Get(r, tokenSession)
		if err != nil {
			da.log.WithError(err).Info("Failed to get existing session, invalidating GitHub login session")
			if err := invalidateGitHubSession(w, r, session); err != nil {
				serverError("Failed to invalidate GitHub session", err)
				return
			}
		}

		// If access token exists, get user login using the access token. This is a
		// chance to validate whether the access token is consumable or not. If
		// not, we invalidate the sessions and continue as if not logged in.
		token, ok := session.Values[tokenKey].(*oauth2.Token)
		var user *gogithub.User
		if ok && token.Valid() {
			goGithubClient := ghclient.NewClient(token.AccessToken, false)
			var err error
			user, err = queryHandler.GetUser(goGithubClient)
			if err != nil {
				if strings.Contains(err.Error(), "401") {
					da.log.Info("Failed to access GitHub with existing access token, invalidating GitHub login session")
					if err := invalidateGitHubSession(w, r, session); err != nil {
						serverError("Failed to invalidate GitHub session", err)
						return
					}
				} else {
					serverError("Error with getting user login", err)
					return
				}
			}
		}

		if user != nil {
			login := *user.Login
			data.Login = true
			// Saves login. We save the login under 2 cookies. One for the use of client to render the
			// data and one encoded for server to verify the identity of the authenticated user.
			http.SetCookie(w, &http.Cookie{
				Name:    loginSession,
				Value:   login,
				Path:    "/",
				Expires: time.Now().Add(time.Hour * 24 * 30),
				Secure:  true,
			})
			session.Values[loginKey] = login
			if err := session.Save(r, w); err != nil {
				serverError("Save oauth session", err)
				return
			}

			// Construct query
			ghc := github.NewClient(func() []byte { return []byte(token.AccessToken) }, githubEndpoint)
			query := da.ConstructSearchQuery(login)
			if err := r.ParseForm(); err == nil {
				if q := r.Form.Get("query"); q != "" {
					query = q
				}
			}
			// If neither repo nor org is specified in the search query. We limit the search to repos that
			// are configured with either Prow or Tide.
			if !queryConstrainsRepos(query) {
				for _, v := range da.repos {
					query += fmt.Sprintf(" repo:\"%s\"", v)
				}
			}
			pullRequests, err := queryHandler.QueryPullRequests(context.Background(), ghc, query)
			if err != nil {
				serverError("Error with querying user data.", err)
				return
			}
			var pullRequestWithContexts []PullRequestWithContext
			for _, pr := range pullRequests {
				prcontext, err := queryHandler.HeadContexts(ghc, pr)
				if err != nil {
					serverError("Error with getting head context of pr", err)
					continue
				}
				pullRequestWithContexts = append(pullRequestWithContexts, PullRequestWithContext{
					Contexts:    prcontext,
					PullRequest: pr,
				})
			}

			data.PullRequestsWithContexts = pullRequestWithContexts
		}

		marshaledData, err := json.Marshal(data)
		if err != nil {
			da.log.WithError(err).Error("Error with marshalling user data.")
		}

		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = ", v)
			w.Write(marshaledData)
			io.WriteString(w, ";")
		} else {
			w.Write(marshaledData)
		}
	}
}

// Query function that returns a list of open pull requests owned by the user whose access token
// is consumed by the github client.
func (da *DashboardAgent) QueryPullRequests(ctx context.Context, ghc githubClient, query string) ([]PullRequest, error) {
	var prs []PullRequest
	vars := map[string]interface{}{
		"query":        (githubql.String)(query),
		"searchCursor": (*githubql.String)(nil),
	}
	var totalCost int
	var remaining int
	for {
		sq := searchQuery{}
		if err := ghc.Query(ctx, &sq, vars); err != nil {
			return nil, err
		}
		totalCost += int(sq.RateLimit.Cost)
		remaining = int(sq.RateLimit.Remaining)
		for _, n := range sq.Search.Nodes {
			prs = append(prs, n.PullRequest)
		}
		if !sq.Search.PageInfo.HasNextPage {
			break
		}
		vars["searchCursor"] = githubql.NewString(sq.Search.PageInfo.EndCursor)
	}
	da.log.Infof("Search for query \"%s\" cost %d point(s). %d remaining.", query, totalCost, remaining)
	return prs, nil
}

func (da *DashboardAgent) HeadContexts(ghc githubClient, pr PullRequest) ([]Context, error) {
	org := string(pr.Repository.Owner.Login)
	repo := string(pr.Repository.Name)
	combined, err := ghc.GetCombinedStatus(org, repo, string(pr.HeadRefOID))
	if err != nil {
		return nil, fmt.Errorf("failed to get the combined status: %v", err)
	}
	contexts := make([]Context, 0, len(combined.Statuses))
	for _, status := range combined.Statuses {
		contexts = append(
			contexts,
			Context{
				Context:     status.Context,
				Description: status.Description,
				State:       strings.ToUpper(status.State),
			},
		)
	}
	return contexts, nil
}

func (da *DashboardAgent) GetUser(client *ghclient.Client) (*gogithub.User, error) {
	return client.GetUser("")
}

func (da *DashboardAgent) ConstructSearchQuery(login string) string {
	tokens := []string{"is:pr", "state:open", "author:" + login}
	for i := range da.repos {
		tokens = append(tokens, fmt.Sprintf("repo:\"%s\"", da.repos[i]))
	}
	return strings.Join(tokens, " ")
}

func queryConstrainsRepos(q string) bool {
	tkns := strings.Split(q, " ")
	for _, tkn := range tkns {
		if strings.HasPrefix(tkn, "org:") || strings.HasPrefix(tkn, "repo:") {
			return true
		}
	}
	return false
}
