// githubqldev is a test program currently being used for developing githubql package.
// Warning: It performs some queries and mutations against real GitHub API.
//
// It's not meant to be a clean or readable example. But it's functional.
// Better, actual examples will be created in the future.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/shurcooL/githubql"
	"golang.org/x/oauth2"
)

func main() {
	flag.Parse()

	err := run()
	if err != nil {
		log.Println(err)
	}
}

func run() error {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_GRAPHQL_TEST_TOKEN")},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	client := githubql.NewClient(httpClient)

	// Query some details about a repository, an issue in it, and its comments.
	{
		type githubqlActor struct {
			Login     githubql.String
			AvatarURL githubql.URI `graphql:"avatarUrl(size:72)"`
			URL       githubql.URI
		}

		var q struct {
			Repository struct {
				DatabaseID githubql.Int
				URL        githubql.URI

				Issue struct {
					Author         githubqlActor
					PublishedAt    githubql.DateTime
					LastEditedAt   *githubql.DateTime
					Editor         *githubqlActor
					Body           githubql.String
					ReactionGroups []struct {
						Content githubql.ReactionContent
						Users   struct {
							Nodes []struct {
								Login githubql.String
							}

							TotalCount githubql.Int
						} `graphql:"users(first:10)"`
						ViewerHasReacted githubql.Boolean
					}
					ViewerCanUpdate githubql.Boolean

					Comments struct {
						Nodes []struct {
							Body   githubql.String
							Author struct {
								Login githubql.String
							}
							Editor struct {
								Login githubql.String
							}
						}
						PageInfo struct {
							EndCursor   githubql.String
							HasNextPage githubql.Boolean
						}
					} `graphql:"comments(first:$commentsFirst,after:$commentsAfter)"`
				} `graphql:"issue(number:$issueNumber)"`
			} `graphql:"repository(owner:$repositoryOwner,name:$repositoryName)"`
			Viewer struct {
				Login      githubql.String
				CreatedAt  githubql.DateTime
				ID         githubql.ID
				DatabaseID githubql.Int
			}
			RateLimit struct {
				Cost      githubql.Int
				Limit     githubql.Int
				Remaining githubql.Int
				ResetAt   githubql.DateTime
			}
		}
		variables := map[string]interface{}{
			"repositoryOwner": githubql.String("shurcooL-test"),
			"repositoryName":  githubql.String("test-repo"),
			"issueNumber":     githubql.Int(1),
			"commentsFirst":   githubql.NewInt(1),
			"commentsAfter":   githubql.NewString("Y3Vyc29yOjE5NTE4NDI1Ng=="),
		}
		err := client.Query(context.Background(), &q, variables)
		if err != nil {
			return err
		}
		printJSON(q)
		//goon.Dump(out)
		//fmt.Println(github.Stringify(out))
	}

	// Toggle a 👍 reaction on an issue.
	//
	// That involves first doing a query (and determining whether the reaction already exists),
	// then either adding or removing it.
	{
		var q struct {
			Repository struct {
				Issue struct {
					ID        githubql.ID
					Reactions struct {
						ViewerHasReacted githubql.Boolean
					} `graphql:"reactions(content:$reactionContent)"`
				} `graphql:"issue(number:$issueNumber)"`
			} `graphql:"repository(owner:$repositoryOwner,name:$repositoryName)"`
		}
		variables := map[string]interface{}{
			"repositoryOwner": githubql.String("shurcooL-test"),
			"repositoryName":  githubql.String("test-repo"),
			"issueNumber":     githubql.Int(2),
			"reactionContent": githubql.ReactionContentThumbsUp,
		}
		err := client.Query(context.Background(), &q, variables)
		if err != nil {
			return err
		}
		fmt.Println("already reacted:", q.Repository.Issue.Reactions.ViewerHasReacted)

		if !q.Repository.Issue.Reactions.ViewerHasReacted {
			// Add reaction.
			var m struct {
				AddReaction struct {
					Subject struct {
						ReactionGroups []struct {
							Content githubql.ReactionContent
							Users   struct {
								TotalCount githubql.Int
							}
						}
					}
				} `graphql:"addReaction(input:$input)"`
			}
			input := githubql.AddReactionInput{
				SubjectID: q.Repository.Issue.ID,
				Content:   githubql.ReactionContentThumbsUp,
			}
			err := client.Mutate(context.Background(), &m, input, nil)
			if err != nil {
				return err
			}
			printJSON(m)
			fmt.Println("Successfully added reaction.")
		} else {
			// Remove reaction.
			var m struct {
				RemoveReaction struct {
					Subject struct {
						ReactionGroups []struct {
							Content githubql.ReactionContent
							Users   struct {
								TotalCount githubql.Int
							}
						}
					}
				} `graphql:"removeReaction(input:$input)"`
			}
			input := githubql.RemoveReactionInput{
				SubjectID: q.Repository.Issue.ID,
				Content:   githubql.ReactionContentThumbsUp,
			}
			err := client.Mutate(context.Background(), &m, input, nil)
			if err != nil {
				return err
			}
			printJSON(m)
			fmt.Println("Successfully removed reaction.")
		}
	}

	return nil
}

// printJSON prints v as JSON encoded with indent to stdout. It panics on any error.
func printJSON(v interface{}) {
	w := json.NewEncoder(os.Stdout)
	w.SetIndent("", "\t")
	err := w.Encode(v)
	if err != nil {
		panic(err)
	}
}
