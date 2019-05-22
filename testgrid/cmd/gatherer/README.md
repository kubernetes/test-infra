#GitHub Issue Gatherer
The GitHub Issue Gatherer collects issues and identifies issues that reference TestGrid tests. This is so that TestGrid can hyperlink failing or flaky tests to open issues.

It has three parts: an Issue Scraper, a Target Identifier, and a Proto Writer.

## Issue Scraper (issuescraper.go)
The Issue Scraper uses the GitHub v3 API to collect every issue in a repository, and every comment left on those issues.

GitHub calls Pull Requests "Issues", but this scraper doesn't.

This scraper uses a caching layer, [ghCache](https://github.com/kubernetes/issues.json/tree/master/ghproxy/ghcache), to minimize repetitive API calls.

The scraper will automatically [depaginate](https://developer.github.com/v3/guides/traversing-with-pagination/) results and store them in a slice of GitHubIssues. That type definition can be found in the code.

## Target Identifier (WIP)

## Proto Writer (WIP)
The Proto Writer writes to [issue_state.proto](https://github.com/kubernetes/test-infra/blob/master/testgrid/issue_state/issue_state.proto), so that TestGrid can make the hyperlink show up.