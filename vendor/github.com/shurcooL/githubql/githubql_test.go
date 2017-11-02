package githubql_test

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/shurcooL/githubql"
)

func TestNewClient_nil(t *testing.T) {
	// Shouldn't panic with nil parameter.
	client := githubql.NewClient(nil)
	_ = client
}

func TestClient_Query(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		if got, want := req.Method, http.MethodPost; got != want {
			t.Errorf("got request method: %v, want: %v", got, want)
		}
		body := mustRead(req.Body)
		if got, want := body, `{"query":"{viewer{login,bio}}"}`+"\n"; got != want {
			t.Errorf("got body: %v, want %v", got, want)
		}
		mustWrite(w, `{"data": {"viewer": {"login": "gopher", "bio": "The Go gopher."}}}`)
	})
	client := githubql.NewClient(&http.Client{Transport: localRoundTripper{mux: mux}})

	type query struct {
		Viewer struct {
			Login     githubql.String
			Biography githubql.String `graphql:"bio"` // GraphQL alias.
		}
	}

	var q query
	err := client.Query(context.Background(), &q, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := q

	var want query
	want.Viewer.Login = "gopher"
	want.Viewer.Biography = "The Go gopher."
	if !reflect.DeepEqual(got, want) {
		t.Errorf("client.Query got: %v, want: %v", got, want)
	}
}

func TestClient_Query_errorResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		mustWrite(w, `{
			"data": null,
			"errors": [
				{
					"message": "Field 'bad' doesn't exist on type 'Query'",
					"locations": [
						{
							"line": 7,
							"column": 3
						}
					]
				}
			]
		}`)
	})
	client := githubql.NewClient(&http.Client{Transport: localRoundTripper{mux: mux}})

	var q struct {
		Bad githubql.String
	}
	err := client.Query(context.Background(), &q, nil)
	if err == nil {
		t.Fatal("got error: nil, want: non-nil")
	}
	if got, want := err.Error(), "Field 'bad' doesn't exist on type 'Query'"; got != want {
		t.Errorf("got error: %v, want: %v", got, want)
	}
}

func TestClient_Query_errorStatusCode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "404 Not Found", http.StatusNotFound)
	})
	client := githubql.NewClient(&http.Client{Transport: localRoundTripper{mux: mux}})

	var q struct {
		Viewer struct {
			Login githubql.String
		}
	}
	err := client.Query(context.Background(), &q, nil)
	if err == nil {
		t.Fatal("got error: nil, want: non-nil")
	}
	if got, want := err.Error(), "unexpected status: 404 Not Found"; got != want {
		t.Errorf("got error: %v, want: %v", got, want)
	}
}

func TestClient_Query_union(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		if got, want := req.Method, http.MethodPost; got != want {
			t.Errorf("got request method: %v, want: %v", got, want)
		}
		body := mustRead(req.Body)
		if got, want := body, `{"query":"query($issueNumber:Int!$repositoryName:String!$repositoryOwner:String!){repository(owner: $repositoryOwner, name: $repositoryName){issue(number: $issueNumber){timeline(first: 10){nodes{__typename,...on ClosedEvent{actor{login},createdAt},...on ReopenedEvent{actor{login},createdAt},...on RenamedTitleEvent{actor{login},createdAt,currentTitle,previousTitle}}}}}}","variables":{"issueNumber":1,"repositoryName":"go","repositoryOwner":"golang"}}`+"\n"; got != want {
			t.Errorf("got body: %v, want %v", got, want)
		}
		mustWrite(w, `{"data": {
			"repository": {
				"issue": {
					"timeline": {
						"nodes": [
							{
								"__typename": "RenamedTitleEvent",
								"createdAt": "2017-06-29T04:12:01Z",
								"actor": {
									"login": "gopher"
								},
								"currentTitle": "new",
								"previousTitle": "old"
							}
						]
					}
				}
			}
		}}`)
	})
	client := githubql.NewClient(&http.Client{Transport: localRoundTripper{mux: mux}})

	type event struct { // Common fields for all events.
		Actor     struct{ Login githubql.String }
		CreatedAt githubql.DateTime
	}
	type issueTimelineItem struct {
		Typename    string `graphql:"__typename"`
		ClosedEvent struct {
			event
		} `graphql:"...on ClosedEvent"`
		ReopenedEvent struct {
			event
		} `graphql:"...on ReopenedEvent"`
		RenamedTitleEvent struct {
			event
			CurrentTitle  string
			PreviousTitle string
		} `graphql:"...on RenamedTitleEvent"`
	}
	type query struct {
		Repository struct {
			Issue struct {
				Timeline struct {
					Nodes []issueTimelineItem
				} `graphql:"timeline(first: 10)"`
			} `graphql:"issue(number: $issueNumber)"`
		} `graphql:"repository(owner: $repositoryOwner, name: $repositoryName)"`
	}

	var q query
	variables := map[string]interface{}{
		"repositoryOwner": githubql.String("golang"),
		"repositoryName":  githubql.String("go"),
		"issueNumber":     githubql.Int(1),
	}
	err := client.Query(context.Background(), &q, variables)
	if err != nil {
		t.Fatal(err)
	}
	got := q

	var want query
	want.Repository.Issue.Timeline.Nodes = make([]issueTimelineItem, 1)
	want.Repository.Issue.Timeline.Nodes[0].Typename = "RenamedTitleEvent"
	want.Repository.Issue.Timeline.Nodes[0].RenamedTitleEvent.Actor.Login = "gopher"
	want.Repository.Issue.Timeline.Nodes[0].RenamedTitleEvent.CreatedAt.Time = time.Unix(1498709521, 0).UTC()
	want.Repository.Issue.Timeline.Nodes[0].RenamedTitleEvent.CurrentTitle = "new"
	want.Repository.Issue.Timeline.Nodes[0].RenamedTitleEvent.PreviousTitle = "old"
	want.Repository.Issue.Timeline.Nodes[0].ClosedEvent.event = want.Repository.Issue.Timeline.Nodes[0].RenamedTitleEvent.event
	want.Repository.Issue.Timeline.Nodes[0].ReopenedEvent.event = want.Repository.Issue.Timeline.Nodes[0].RenamedTitleEvent.event
	if !reflect.DeepEqual(got, want) {
		t.Errorf("client.Query:\ngot:  %+v\nwant: %+v", got, want)
	}
}

func TestClient_Mutate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		if got, want := req.Method, http.MethodPost; got != want {
			t.Errorf("got request method: %v, want: %v", got, want)
		}
		body := mustRead(req.Body)
		if got, want := body, `{"query":"mutation($input:AddReactionInput!){addReaction(input:$input){reaction{content},subject{id,reactionGroups{users{totalCount}}}}}","variables":{"input":{"subjectId":"MDU6SXNzdWUyMTc5NTQ0OTc=","content":"HOORAY"}}}`+"\n"; got != want {
			t.Errorf("got body: %v, want %v", got, want)
		}
		mustWrite(w, `{"data": {
			"addReaction": {
				"reaction": {
					"content": "HOORAY"
				},
				"subject": {
					"id": "MDU6SXNzdWUyMTc5NTQ0OTc=",
					"reactionGroups": [
						{
							"users": {"totalCount": 3}
						}
					]
				}
			}
		}}`)
	})
	client := githubql.NewClient(&http.Client{Transport: localRoundTripper{mux: mux}})

	type reactionGroup struct {
		Users struct {
			TotalCount githubql.Int
		}
	}
	type mutation struct {
		AddReaction struct {
			Reaction struct {
				Content githubql.ReactionContent
			}
			Subject struct {
				ID             githubql.ID
				ReactionGroups []reactionGroup
			}
		} `graphql:"addReaction(input:$input)"`
	}

	var m mutation
	input := githubql.AddReactionInput{
		SubjectID: "MDU6SXNzdWUyMTc5NTQ0OTc=",
		Content:   githubql.ReactionContentHooray,
	}
	err := client.Mutate(context.Background(), &m, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := m

	var want mutation
	want.AddReaction.Reaction.Content = githubql.ReactionContentHooray
	want.AddReaction.Subject.ID = "MDU6SXNzdWUyMTc5NTQ0OTc="
	var rg reactionGroup
	rg.Users.TotalCount = 3
	want.AddReaction.Subject.ReactionGroups = []reactionGroup{rg}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("client.Query got: %v, want: %v", got, want)
	}
}

// localRoundTripper is an http.RoundTripper that executes HTTP transactions
// by using mux directly, instead of going over an HTTP connection.
type localRoundTripper struct {
	mux *http.ServeMux
}

func (l localRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	l.mux.ServeHTTP(w, req)
	return w.Result(), nil
}

func mustRead(r io.Reader) string {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func mustWrite(w io.Writer, s string) {
	_, err := io.WriteString(w, s)
	if err != nil {
		panic(err)
	}
}
