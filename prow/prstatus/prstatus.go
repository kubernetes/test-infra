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

	"github.com/gorilla/sessions"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/githuboauth"
)

const (
	loginSession = "github_login"
	tokenSession = "access-token-session"
	tokenKey     = "access-token"
	loginKey     = "login"
)

// pullRequestQueryHandler defines an interface that query handlers should implement.
type pullRequestQueryHandler interface {
	queryPullRequests(context.Context, githubQuerier, string) ([]PullRequest, error)
	getHeadContexts(ghc githubStatusFetcher, pr PullRequest) ([]Context, error)
}

// UserData represents data returned to client request to the endpoint. It has a flag that indicates
// whether the user has logged in his github or not and list of open pull requests owned by the
// user.
type UserData struct {
	Login                    bool
	PullRequestsWithContexts []PullRequestWithContexts
}

// PullRequestWithContexts contains a pull request with its latest commit contexts.
type PullRequestWithContexts struct {
	Contexts    []Context
	PullRequest PullRequest
}

// DashboardAgent is responsible for handling request to /pr-status endpoint.
// It will serve a list of open pull requests owned by the user.
type DashboardAgent struct {
	repos  []string
	goac   *githuboauth.Config
	github flagutil.GitHubOptions

	log *logrus.Entry
}

// Label represents a GitHub label.
type Label struct {
	ID   githubql.ID
	Name githubql.String
}

// Context represent a GitHub status check context.
type Context struct {
	Context     string
	Description string
	State       string
}

// PullRequest holds the GraphQL response data for a GitHub pull request.
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

// UserLoginQuery holds the GraphQL query for the currently authenticated user.
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

// NewDashboardAgent creates a new user dashboard agent .
func NewDashboardAgent(repos []string, config *githuboauth.Config, github *flagutil.GitHubOptions, log *logrus.Entry) *DashboardAgent {
	return &DashboardAgent{
		repos:  repos,
		goac:   config,
		github: *github,
		log:    log,
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

type GitHubClient interface {
	githubQuerier
	githubStatusFetcher
	BotUser() (*github.UserData, error)
}

type githubClientCreator func(accessToken string) GitHubClient

// HandlePrStatus returns a http handler function that handles request to /pr-status
// endpoint. The handler takes user access token stored in the cookie to query to GitHub on behalf
// of the user and serve the data in return. The Query handler is passed to the method so as it
// can be mocked in the unit test..
func (da *DashboardAgent) HandlePrStatus(queryHandler pullRequestQueryHandler, createClient githubClientCreator) http.HandlerFunc {
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
		token, ok := session.Values[tokenKey].(*oauth2.Token) // TODO(fejta): client cache
		var user *github.User
		var botUser *github.UserData
		if ok && token.Valid() {
			githubClient := createClient(token.AccessToken)
			var err error
			botUser, err = githubClient.BotUser()
			user = &github.User{Login: botUser.Login}
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
			login := user.Login
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
			ghc := da.github.GitHubClientWithAccessToken(token.AccessToken) // TODO(fejta): we should not recreate the client
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
			pullRequests, err := queryHandler.queryPullRequests(context.Background(), ghc, query)
			if err != nil {
				serverError("Error with querying user data.", err)
				return
			}
			var pullRequestWithContexts []PullRequestWithContexts
			for _, pr := range pullRequests {
				prcontexts, err := queryHandler.getHeadContexts(ghc, pr)
				if err != nil {
					serverError("Error with getting head context of pr", err)
					continue
				}
				pullRequestWithContexts = append(pullRequestWithContexts, PullRequestWithContexts{
					Contexts:    prcontexts,
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

type githubQuerier interface {
	Query(context.Context, interface{}, map[string]interface{}) error
}

// queryPullRequests is a query function that returns a list of open pull requests owned by the user whose access token
// is consumed by the github client.
func (da *DashboardAgent) queryPullRequests(ctx context.Context, ghc githubQuerier, query string) ([]PullRequest, error) {
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
			org := string(n.PullRequest.Repository.Owner.Login)
			repo := string(n.PullRequest.Repository.Name)
			ref := string(n.PullRequest.HeadRefOID)
			if org == "" || repo == "" || ref == "" {
				da.log.Warningf("Skipped empty pull request returned by query \"%s\": %v", query, n.PullRequest)
				continue
			}
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

type githubStatusFetcher interface {
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
	ListCheckRuns(org, repo, ref string) (*github.CheckRunList, error)
}

// getHeadContexts returns the status checks' contexts of the head commit of the PR.
func (da *DashboardAgent) getHeadContexts(ghc githubStatusFetcher, pr PullRequest) ([]Context, error) {
	org := string(pr.Repository.Owner.Login)
	repo := string(pr.Repository.Name)
	combined, err := ghc.GetCombinedStatus(org, repo, string(pr.HeadRefOID))
	if err != nil {
		return nil, fmt.Errorf("failed to get the combined status: %v", err)
	}
	checkruns, err := ghc.ListCheckRuns(org, repo, string(pr.HeadRefOID))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch checkruns: %v", err)
	}
	contexts := make([]Context, 0, len(combined.Statuses)+len(checkruns.CheckRuns))
	for _, status := range combined.Statuses {
		contexts = append(contexts, Context{
			Context:     status.Context,
			Description: status.Description,
			State:       strings.ToUpper(status.State),
		})
	}
	for _, checkrun := range checkruns.CheckRuns {
		var state string
		if checkrun.CompletedAt == "" {
			state = "PENDING"
		} else if strings.ToUpper(checkrun.Conclusion) == "NEUTRAL" {
			state = "SUCCESS"
		} else {
			state = strings.ToUpper(checkrun.Conclusion)
		}
		contexts = append(contexts, Context{
			Context:     checkrun.Name,
			Description: checkrun.DetailsURL,
			State:       state,
		})
	}
	return contexts, nil
}

// ConstructSearchQuery returns the GitHub search query string for PRs that are open and authored
// by the user passed. The search is scoped to repositories that are configured with either Prow or
// Tide.
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
