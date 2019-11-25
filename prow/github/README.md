# GitHub API Library

This GitHub API library is used by multiple parts of Prow.
It uses both [v3](https://developer.github.com/v3/) and [v4](https://developer.github.com/v4/)
of GitHub's API. It is subject to change as needed without notice, but you can reuse and extend it
within this repository.

Its primary component is [client.go](client.go), a GitHub client that sends and receives API calls.

## Recommended Usage

### Instantiation
An application that takes flags may want to set GitHub flags, such as a proxy endpoint. To do that,
[GitHubOptions](../flagutil/github.go) has a method that returns a GitHub client.

If you're not using flags, you can instantiate a client with the `NewClient` and
`NewClientWithFields` methods

### Interfacing a Subset of Client
This client has a lot of functions listed in the interfaces of [client.go](client.go). Further,
these interfaces may change at any time. To avoid having to extend the entire interface, we
recommend writing a local interface that uses the functionality you need.

For example, if you only need to get and edit issues, you might write an interface like the
following:

```golang
type githubClient interface {
	GetIssue(org, repo string, number int) (*github.Issue, error)
	EditIssue(org, repo string, number int, issue *github.Issue) (*github.Issue, error)
}
```

The provided fake works like this; [FakeClient](fakegithub/fakegithub.go) doesn't completely
implement Client, but gives many common functions used in testing.