# Gerrit

[Gerrit] is a free, web-based team code collaboration tool.

## Related Deployments

[Adapter](/prow/cmd/gerrit)

[Reporter](/prow/cmd/crier) 

## Related packages

#### Client

We have a [gerrit-client package](/prow/gerrit/client) that provides a thin wrapper around  
[andygrunwald/go-gerrit](https://github.com/andygrunwald/go-gerrit), which is a go client library
for accessing the [Gerrit Code Review REST API](https://gerrit-review.googlesource.com/Documentation/rest-api.html)

You can create a client instance by pass in a map of instance-name:project-ids, and pass in an oauth token path to
start the client, like:

```go
projects := map[string][]string{
	"foo.googlesource.com": {
		"project-bar",
		"project-baz",
	},
}

c, err := gerrit.NewClient(projects)
if err != nil {
	// handle error
}
c.Start(cookiefilePath)
```

The client will try to refetch token from the path every 10 minutes.

You should also utilize [grandmatriarch] to generate a token from a
passed-in service account credential.

If you need extra features, feel free to introduce new gerrit API functions to the client package.


#### Adapter

The adapter package implements a controller that is periodically polling gerrit, and triggering
presubmit jobs base on your prow config.


#### Reporter

The reporter package handles send presubmit results back to gerrit. It implements a reporter interface
from [crier].

Currently it will set a review message on target patchset for each finished presubmit, with a URL field
which you can configure in your prow configuration.

<!-- TODO(krzyzacy): add labeling + aggregate result after https://github.com/kubernetes/test-infra/pull/9506 -->

## Caveat

The gerrit adapter currently does not support [gerrit hooks](https://gerrit-review.googlesource.com/Documentation/config-hooks.html),
If you need them, please send us a PR to support them :-)


[Gerrit]: https://www.gerritcodereview.com/
[Prow]: /prow/README.md
[grandmatriarch]: /prow/cmd/grandmatriarch
[crier]: /prow/crier
