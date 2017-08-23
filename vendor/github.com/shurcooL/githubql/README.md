githubql
========

[![Build Status](https://travis-ci.org/shurcooL/githubql.svg?branch=master)](https://travis-ci.org/shurcooL/githubql) [![GoDoc](https://godoc.org/github.com/shurcooL/githubql?status.svg)](https://godoc.org/github.com/shurcooL/githubql)

Package `githubql` is a client library for accessing GitHub GraphQL API v4 (https://developer.github.com/v4/).

If you're looking for a client library for GitHub REST API v3, the recommended package is [`github.com/google/go-github/github`](https://godoc.org/github.com/google/go-github/github).

**Status:** In active early research and development. The API will change when opportunities for improvement are discovered; it is not yet frozen.

Focus
-----

-	Friendly, simple and powerful API.
-	Correctness, high performance and efficiency.
-	Support all of GitHub GraphQL API v4 via code generation from schema.

### Roadmap

Currently implemented:

-	[x] Basic and intermediate queries.
-	[x] All mutations.
-	[x] Query minification before network transfer.
-	[x] Scalars.
	-	[ ] Improved support (https://github.com/shurcooL/githubql/issues/9).
-	[x] Specifying arguments and passing variables.
-	[x] Thorough test coverage.
	-	[x] Initial basic tests (hacky but functional).
	-	[x] Better organized, medium sized tests.
-	[x] Aliases.
	-	[ ] Documentation.
	-	[x] Improved support.
-	[x] [Inline fragments](http://graphql.org/learn/queries/#inline-fragments).
-	[x] Generate all of objects, enums, input objects, etc.
	-	[x] Clean up GitHub documentation to pass `golint`.
-	[x] Unions.
	-	[x] Functional.
	-	[x] Improved support (https://github.com/shurcooL/githubql/issues/10).
-	[ ] Directives (haven't tested yet, but expect it to be supported).
-	[ ] Research and complete, document the rest of GraphQL features.
-	[ ] Fully document (and add tests for edge cases) the `graphql` struct field tag.
-	[ ] Extremely clean, beautiful, idiomatic Go code (100% coverage, 0 lines of hacky code).
	-	[x] Document all public identifiers.
	-	[ ] Clean up implementations of some private helpers (currently functional, but hacky).

Future:

-	[ ] GitHub Enterprise support (when it's available; GitHub themselves haven't released support yet, see [here](https://platform.github.community/t/is-graphql-available-for-github-enterprise/1224)).
-	[ ] Frontend support (e.g., using `githubql` in frontend Go code via [GopherJS](https://github.com/gopherjs/gopherjs)).
-	[ ] Local error detection (maybe).
-	[ ] Calculating a rate limit score before running the call.
-	[ ] Avoiding making network calls when rate limit quota exceeded and not yet reset.
-	[ ] Support for OpenTracing.

Known unknowns:

-	Whether or not the current API design will scale to support all of advanced GraphQL specification features, and future changes. So far, things are looking great, no major blockers found. I am constantly evaluating it against alternative API designs that I've considered and prototyped myself, and new ones that I become aware of.
-	I have only explored roughly 80% of the GraphQL specification (Working Draft – October 2016).
-	Performance, allocations, memory usage under heavy workloads in long-running processes.
-	Optimal long-term package/code layout (i.e., whether to split off some of the parts into smaller sub-packages).

Installation
------------

`githubql` requires Go version 1.8 or later.

```bash
go get -u github.com/shurcooL/githubql
```

Usage
-----

### Authentication

GitHub GraphQL API v4 [requires authentication](https://developer.github.com/v4/guides/forming-calls/#authenticating-with-graphql). The `githubql` package does not directly handle authentication. Instead, when creating a new client, you're expected to pass an `http.Client` that performs authentication. The easiest and recommended way to do this is to use the [`golang.org/x/oauth2`](https://golang.org/x/oauth2) package. You'll need an OAuth token from GitHub (for example, a [personal API token](https://help.github.com/articles/creating-a-personal-access-token-for-the-command-line/)) with the right scopes. Then:

```Go
import "golang.org/x/oauth2"

func main() {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	httpClient := oauth2.NewClient(context.Background(), src)

	client := githubql.NewClient(httpClient)
	// Use client...
```

### Simple Query

To make a query, you need to define a Go type that corresponds to the GitHub GraphQL schema, and contains the fields you're interested in querying. You can look up the GitHub GraphQL schema at https://developer.github.com/v4/reference/query/.

For example, to make the following GraphQL query:

```GraphQL
query {
	viewer {
		login
		createdAt
	}
}
```

You can define this variable:

```Go
var query struct {
	Viewer struct {
		Login     githubql.String
		CreatedAt githubql.DateTime
	}
}
```

And call `client.Query`, passing a pointer to it:

```Go
err := client.Query(context.Background(), &query, nil)
if err != nil {
	// Handle error.
}
fmt.Println("    Login:", query.Viewer.Login)
fmt.Println("CreatedAt:", query.Viewer.CreatedAt)

// Output:
//     Login: gopher
// CreatedAt: 2017-05-26 21:17:14 +0000 UTC
```

### Arguments and Variables

Often, you'll want to specify arguments on some fields. You can use the `graphql` struct field tag for this.

For example, to make the following GraphQL query:

```GraphQL
{
	repository(owner: "octocat", name: "Hello-World") {
		description
	}
}
```

You can define this variable:

```Go
var q struct {
	Repository struct {
		Description githubql.String
	} `graphql:"repository(owner: \"octocat\", name: \"Hello-World\")"`
}
```

And call `client.Query`:

```Go
err := client.Query(context.Background(), &q, nil)
if err != nil {
	// Handle error.
}
fmt.Println(q.Repository.Description)

// Output:
// My first repository on GitHub!
```

However, that'll only work if the arguments are constant and known in advance. Otherwise, you will need to make use of variables. Replace the constants in the struct field tag with variable names:

```Go
// fetchRepoDescription fetches description of repo with owner and name.
func fetchRepoDescription(ctx context.Context, owner, name string) (string, error) {
	var q struct {
		Repository struct {
			Description githubql.String
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
```

Then, define a `variables` map with their values:

```Go
	variables := map[string]interface{}{
		"owner": githubql.String(owner),
		"name":  githubql.String(name),
	}
```

Finally, call `client.Query` providing `variables`:

```Go
	err := client.Query(ctx, &q, variables)
	return string(q.Repository.Description), err
}
```

### Pagination

Imagine you wanted to get a complete list of comments in an issue, and not just the first 10 or so. To do that, you'll need to perform multiple queries and use pagination information. For example:

```Go
type comment struct {
	Body   githubql.String
	Author struct {
		Login     githubql.String
		AvatarURL githubql.URI `graphql:"avatarUrl(size: 72)"`
	}
	ViewerCanReact githubql.Boolean
}
var q struct {
	Repository struct {
		Issue struct {
			Comments struct {
				Nodes    []comment
				PageInfo struct {
					EndCursor   githubql.String
					HasNextPage githubql.Boolean
				}
			} `graphql:"comments(first: 100, after: $commentsCursor)"` // 100 per page.
		} `graphql:"issue(number: $issueNumber)"`
	} `graphql:"repository(owner: $repositoryOwner, name: $repositoryName)"`
}
variables := map[string]interface{}{
	"repositoryOwner": githubql.String(owner),
	"repositoryName":  githubql.String(name),
	"issueNumber":     githubql.Int(issue),
	"commentsCursor":  (*githubql.String)(nil), // Null after argument to get first page.
}

// Get comments from all pages.
var allComments []comment
for {
	err := s.clQL.Query(ctx, &q, variables)
	if err != nil {
		return err
	}
	allComments = append(allComments, q.Repository.Issue.Comments.Nodes...)
	if !q.Repository.Issue.Comments.PageInfo.HasNextPage {
		break
	}
	variables["commentsCursor"] = githubql.NewString(q.Repository.Issue.Comments.PageInfo.EndCursor)
}
```

There is more than one way to perform pagination. Consider additional fields inside [`PageInfo`](https://developer.github.com/v4/reference/object/pageinfo/) object.

### Mutations

Mutations often require information that you can only find out by performing a query first. Let's suppose you've already done that.

For example, to make the following GraphQL mutation:

```GraphQL
mutation($input: AddReactionInput!) {
	addReaction(input: $input) {
		reaction {
			content
		}
		subject {
			id
		}
	}
}
variables {
	"input": {
		"subjectId": "MDU6SXNzdWUyMTc5NTQ0OTc=",
		"content": "HOORAY"
	}
}
```

You can define:

```Go
var m struct {
	AddReaction struct {
		Reaction struct {
			Content githubql.ReactionContent
		}
		Subject struct {
			ID githubql.ID
		}
	} `graphql:"addReaction(input: $input)"`
}
input := githubql.AddReactionInput{
	SubjectID: targetIssue.ID, // ID of the target issue from a previous query.
	Content:   githubql.Hooray,
}
```

And call `client.Mutate`:

```Go
err := client.Mutate(context.Background(), &m, input, nil)
if err != nil {
	// Handle error.
}
fmt.Printf("Added a %v reaction to subject with ID %#v!\n", m.AddReaction.Reaction.Content, m.AddReaction.Subject.ID)

// Output:
// Added a HOORAY reaction to subject with ID "MDU6SXNzdWUyMTc5NTQ0OTc="!
```

Directories
-----------

| Path                                                                                      | Synopsis                                                                            |
|-------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------|
| [example/githubqldev](https://godoc.org/github.com/shurcooL/githubql/example/githubqldev) | githubqldev is a test program currently being used for developing githubql package. |

License
-------

-	[MIT License](LICENSE)
