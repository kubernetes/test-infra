/*
Copyright 2019 The Kubernetes Authors.

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

package ghmetrics

import "testing"

func Test_GetSimplifiedPath(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "path not in tree", args: args{path: "/this/path/is/not/in/the/tree"}, want: "unmatched"},
		{name: "path not in tree #2", args: args{path: "/repos/hello/world/its/a/party"}, want: "unmatched"},
		{name: "path not in tree #3", args: args{path: "/path-not-handled"}, want: "unmatched"},

		{name: "repo branches protection (restrictions for users) by name ", args: args{path: "/repos/testOwner/testRepo/branches/testBranch/protection/restrictions/users"}, want: "/repos/:owner/:repo/branches/:branch/protection/restrictions/users"},
		{name: "repositories", args: args{path: "/repositories"}, want: "/repositories"},

		{name: "user", args: args{path: "/user"}, want: "/user"},
		{name: "users", args: args{path: "/users"}, want: "/users"},
		{name: "user by username", args: args{path: "/users/testUser"}, want: "/users/:username"},

		{name: "orgs", args: args{path: "/orgs"}, want: "/orgs"},
		{name: "org by orgname", args: args{path: "/orgs/testOrg"}, want: "/orgs/:orgname"},

		{name: "issues", args: args{path: "/issues"}, want: "/issues"},
		{name: "issues by id", args: args{path: "/issues/testId"}, want: "/issues/:issueId"},

		{name: "search", args: args{path: "/search"}, want: "/search"},
		{name: "search repositories", args: args{path: "/search/repositories"}, want: "/search/repositories"},
		{name: "search commits", args: args{path: "/search/commits"}, want: "/search/commits"},
		{name: "search code", args: args{path: "/search/code"}, want: "/search/code"},
		{name: "search issues", args: args{path: "/search/issues"}, want: "/search/issues"},
		{name: "search users", args: args{path: "/search/users"}, want: "/search/users"},
		{name: "search topics", args: args{path: "/search/topics"}, want: "/search/topics"},
		{name: "search labels", args: args{path: "/search/labels"}, want: "/search/labels"},

		{name: "gists", args: args{path: "/gists"}, want: "/gists"},
		{name: "gists public", args: args{path: "/gists/public"}, want: "/gists/public"},
		{name: "gists starred", args: args{path: "/gists/starred"}, want: "/gists/starred"},

		{name: "notifications", args: args{path: "/notifications"}, want: "/notifications"},

		{name: "graphql", args: args{path: "/graphql"}, want: "/graphql"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simplifier.Simplify(tt.args.path); got != tt.want {
				t.Errorf("GetSimplifiedPath(%s) = %v, want %v", tt.args.path, got, tt.want)
			}
		})
	}
}

func Test_GetSimplifiedPathRepos(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "access repos/ path should not fail, is not explicitly handled", args: args{path: "/repos"}, want: "/repos"},
		{name: "access repos/ path should not fail, is not explicitly handled", args: args{path: "/repos/testOwner/testRepo/"}, want: "unmatched"},

		{name: "repo issues", args: args{path: "/repos/testOwner/testRepo/issues"}, want: "/repos/:owner/:repo/issues"},
		{name: "repo issue by number", args: args{path: "/repos/testOwner/testRepo/issues/21342"}, want: "/repos/:owner/:repo/issues/:issueId"},
		{name: "repo issue by number lock", args: args{path: "/repos/testOwner/testRepo/issues/21321/lock"}, want: "/repos/:owner/:repo/issues/:issueId/lock"},
		{name: "repo issues comments", args: args{path: "/repos/testOwner/testRepo/issues/comments"}, want: "/repos/:owner/:repo/issues/comments"},
		{name: "repo issues comment by number", args: args{path: "/repos/testOwner/testRepo/issues/comments/321"}, want: "/repos/:owner/:repo/issues/comments/:commentId"},
		{name: "repo issues events", args: args{path: "/repos/testOwner/testRepo/issues/events"}, want: "/repos/:owner/:repo/issues/events"},
		{name: "repo issues event by number", args: args{path: "/repos/testOwner/testRepo/issues/events/123"}, want: "/repos/:owner/:repo/issues/events/:eventId"},

		{name: "repo keys", args: args{path: "/repos/testOwner/testRepo/keys"}, want: "/repos/:owner/:repo/keys"},
		{name: "repo key by id", args: args{path: "/repos/testOwner/testRepo/keys/421"}, want: "/repos/:owner/:repo/keys/:keyId"},

		{name: "repo labels", args: args{path: "/repos/testOwner/testRepo/labels"}, want: "/repos/:owner/:repo/labels"},
		{name: "repo label by name", args: args{path: "/repos/testOwner/testRepo/labels/testLabel"}, want: "/repos/:owner/:repo/labels/:labelId"},

		{name: "repo merges", args: args{path: "/repos/testOwner/testRepo/merges"}, want: "/repos/:owner/:repo/merges"},

		{name: "repo milestones", args: args{path: "/repos/testOwner/testRepo/milestones"}, want: "/repos/:owner/:repo/milestones"},
		{name: "repo milestones by number", args: args{path: "/repos/testOwner/testRepo/milestones/421"}, want: "/repos/:owner/:repo/milestones/:milestone"},

		{name: "repo pulls", args: args{path: "/repos/testOwner/testRepo/pulls"}, want: "/repos/:owner/:repo/pulls"},
		{name: "repo pulls by number", args: args{path: "/repos/testOwner/testRepo/pulls/421"}, want: "/repos/:owner/:repo/pulls/:pullId"},

		{name: "repo releases", args: args{path: "/repos/testOwner/testRepo/releases"}, want: "/repos/:owner/:repo/releases"},
		{name: "repo releases by number", args: args{path: "/repos/testOwner/testRepo/releases/421"}, want: "/repos/:owner/:repo/releases/:releaseId"},

		{name: "repo stargazers", args: args{path: "/repos/testOwner/testRepo/stargazers"}, want: "/repos/:owner/:repo/stargazers"},

		{name: "repo statuses", args: args{path: "/repos/testOwner/testRepo/statuses"}, want: "/repos/:owner/:repo/statuses"},
		{name: "repo statuses by sha", args: args{path: "/repos/testOwner/testRepo/statuses/4u8dsaag89ewfdjkt9fdajdsa"}, want: "/repos/:owner/:repo/statuses/:statusId"},

		{name: "repo subscribers", args: args{path: "/repos/testOwner/testRepo/subscribers"}, want: "/repos/:owner/:repo/subscribers"},

		{name: "repo subscribers", args: args{path: "/repos/testOwner/testRepo/subscribers"}, want: "/repos/:owner/:repo/subscribers"},

		{name: "repo notifications", args: args{path: "/repos/testOwner/testRepo/notifications"}, want: "/repos/:owner/:repo/notifications"},

		{name: "repo branches", args: args{path: "/repos/testOwner/testRepo/branches"}, want: "/repos/:owner/:repo/branches"},
		{name: "repo branches by name", args: args{path: "/repos/testOwner/testRepo/branches/testBranch"}, want: "/repos/:owner/:repo/branches/:branch"},
		{name: "repo branches protection by name ", args: args{path: "/repos/testOwner/testRepo/branches/testBranch/protection"}, want: "/repos/:owner/:repo/branches/:branch/protection"},
		{name: "repo branches protection (required status checks) by name ", args: args{path: "/repos/testOwner/testRepo/branches/testBranch/protection/required_status_checks"}, want: "/repos/:owner/:repo/branches/:branch/protection/required_status_checks"},
		{name: "repo branches protection (required status checks, contexts) by name ", args: args{path: "/repos/testOwner/testRepo/branches/testBranch/protection/required_status_checks/contexts"}, want: "/repos/:owner/:repo/branches/:branch/protection/required_status_checks/contexts"},
		{name: "repo branches protection (required pull request reviews) by name ", args: args{path: "/repos/testOwner/testRepo/branches/testBranch/protection/required_pull_request_reviews"}, want: "/repos/:owner/:repo/branches/:branch/protection/required_pull_request_reviews"},
		{name: "repo branches protection (required signatures) by name ", args: args{path: "/repos/testOwner/testRepo/branches/testBranch/protection/required_signatures"}, want: "/repos/:owner/:repo/branches/:branch/protection/required_signatures"},
		{name: "repo branches protection (enforce admins) by name ", args: args{path: "/repos/testOwner/testRepo/branches/testBranch/protection/enforce_admins"}, want: "/repos/:owner/:repo/branches/:branch/protection/enforce_admins"},
		{name: "repo branches protection (restrictions) by name ", args: args{path: "/repos/testOwner/testRepo/branches/testBranch/protection/restrictions"}, want: "/repos/:owner/:repo/branches/:branch/protection/restrictions"},
		{name: "repo branches protection (restrictions for teams) by name ", args: args{path: "/repos/testOwner/testRepo/branches/testBranch/protection/restrictions/teams"}, want: "/repos/:owner/:repo/branches/:branch/protection/restrictions/teams"},
		{name: "repo branches protection (restrictions for users) by name ", args: args{path: "/repos/testOwner/testRepo/branches/testBranch/protection/restrictions/users"}, want: "/repos/:owner/:repo/branches/:branch/protection/restrictions/users"},

		{name: "repo archive", args: args{path: "/repos/testOwner/testRepo/archive"}, want: "/repos/:owner/:repo/archive"},
		{name: "repo archive ref", args: args{path: "/repos/testOwner/testRepo/archive/tar.gz"}, want: "/repos/:owner/:repo/archive/:zip"},

		{name: "repo assignees", args: args{path: "/repos/testOwner/testRepo/assignees"}, want: "/repos/:owner/:repo/assignees"},
		{name: "repo assignees by name", args: args{path: "/repos/testOwner/testRepo/assignees/testUser"}, want: "/repos/:owner/:repo/assignees/:assigneeId"},

		{name: "repo git commits", args: args{path: "/repos/testOwner/testRepo/git/commits"}, want: "/repos/:owner/:repo/git/commits"},
		{name: "repo git commit by sha", args: args{path: "/repos/testOwner/testRepo/git/commits/4u8dsaag89ewfdjkt9fdajdsa"}, want: "/repos/:owner/:repo/git/commits/:sha"},
		{name: "repo git refs", args: args{path: "/repos/testOwner/testRepo/git/ref"}, want: "/repos/:owner/:repo/git/ref"},
		{name: "repo git ref by sha", args: args{path: "/repos/testOwner/testRepo/git/ref/4u8dsaag89ewfdjkt9fdajdsa"}, want: "/repos/:owner/:repo/git/ref/:refId"},
		{name: "repo git tags", args: args{path: "/repos/testOwner/testRepo/git/tags"}, want: "/repos/:owner/:repo/git/tags"},
		{name: "repo git tag by sha", args: args{path: "/repos/testOwner/testRepo/git/tags/4u8dsaag89ewfdjkt9fdajdsa"}, want: "/repos/:owner/:repo/git/tags/:tagId"},
		{name: "repo git trees", args: args{path: "/repos/testOwner/testRepo/git/trees"}, want: "/repos/:owner/:repo/git/trees"},
		{name: "repo git tree by sha", args: args{path: "/repos/testOwner/testRepo/git/trees/4u8dsaag89ewfdjkt9fdajdsa"}, want: "/repos/:owner/:repo/git/trees/:sha"},

		{name: "repo git tags", args: args{path: "/repos/testOwner/testRepo/hooks"}, want: "/repos/:owner/:repo/hooks"},

		{name: "repo collaborators", args: args{path: "/repos/testOwner/testRepo/collaborators"}, want: "/repos/:owner/:repo/collaborators"},
		{name: "repo collaborators by name", args: args{path: "/repos/testOwner/testRepo/collaborators/testCollaborator"}, want: "/repos/:owner/:repo/collaborators/:collaboratorId"},

		{name: "repo comments", args: args{path: "/repos/testOwner/testRepo/comments"}, want: "/repos/:owner/:repo/comments"},
		{name: "repo comments by id", args: args{path: "/repos/testOwner/testRepo/comments/testComment"}, want: "/repos/:owner/:repo/comments/:commentId"},

		{name: "repo commits", args: args{path: "/repos/testOwner/testRepo/commits"}, want: "/repos/:owner/:repo/commits"},
		{name: "repo commits by sha", args: args{path: "/repos/testOwner/testRepo/commits/testCommitSha"}, want: "/repos/:owner/:repo/commits/:sha"},
		{name: "repo commit status by sha", args: args{path: "/repos/testOwner/testRepo/commits/testCommitSha/status"}, want: "/repos/:owner/:repo/commits/:sha/status"},
		{name: "longer postfix is unmatched", args: args{path: "/repos/testOwner/testRepo/commits/testCommitSha/status/else"}, want: "unmatched"},

		// /compare/base...head
		{name: "repo compare", args: args{path: "/repos/testOwner/testRepo/compare/testBase...testHead"}, want: "/repos/:owner/:repo/compare/:sha"},

		{name: "repo contents", args: args{path: "/repos/testOwner/testRepo/contents"}, want: "/repos/:owner/:repo/contents"},
		{name: "repo contents by name", args: args{path: "/repos/testOwner/testRepo/contents/testContents"}, want: "/repos/:owner/:repo/contents/:contentId"},

		{name: "repo deployments", args: args{path: "/repos/testOwner/testRepo/deployments"}, want: "/repos/:owner/:repo/deployments"},

		{name: "repo downloads", args: args{path: "/repos/testOwner/testRepo/downloads"}, want: "/repos/:owner/:repo/downloads"},

		{name: "repo events", args: args{path: "/repos/testOwner/testRepo/events"}, want: "/repos/:owner/:repo/events"},

		{name: "repo forks", args: args{path: "/repos/testOwner/testRepo/forks"}, want: "/repos/:owner/:repo/forks"},

		{name: "repo topics", args: args{path: "/repos/testOwner/testRepo/topics"}, want: "/repos/:owner/:repo/topics"},

		{name: "repo vulnerability-alerts", args: args{path: "/repos/testOwner/testRepo/vulnerability-alerts"}, want: "/repos/:owner/:repo/vulnerability-alerts"},

		{name: "repo automated-security-fixes", args: args{path: "/repos/testOwner/testRepo/automated-security-fixes"}, want: "/repos/:owner/:repo/automated-security-fixes"},

		{name: "repo contributors", args: args{path: "/repos/testOwner/testRepo/contributors"}, want: "/repos/:owner/:repo/contributors"},

		{name: "repo languages", args: args{path: "/repos/testOwner/testRepo/languages"}, want: "/repos/:owner/:repo/languages"},

		{name: "repo teams", args: args{path: "/repos/testOwner/testRepo/teams"}, want: "/repos/:owner/:repo/teams"},

		{name: "repo tags", args: args{path: "/repos/testOwner/testRepo/tags"}, want: "/repos/:owner/:repo/tags"},

		{name: "repo transfer", args: args{path: "/repos/testOwner/testRepo/transfer"}, want: "/repos/:owner/:repo/transfer"},

		{name: "master ref", args: args{path: "/repos/cri-o/cri-o/git/refs/heads/master"}, want: "/repos/:owner/:repo/git/refs/heads/:ref"},
		{name: "issue comments", args: args{path: "/repos/openshift/aws-account-operator/issues/104/comments"}, want: "/repos/:owner/:repo/issues/:issueId/comments"},
		{name: "issue labels", args: args{path: "/repos/openshift/aws-account-operator/issues/104/labels"}, want: "/repos/:owner/:repo/issues/:issueId/labels"},
		{name: "issue label", args: args{path: "/repos/openshift/aws-account-operator/issues/104/labels/needs-rebase"}, want: "/repos/:owner/:repo/issues/:issueId/labels/:labelId"},
		{name: "issue events", args: args{path: "/repos/helm/charts/issues/15756/events"}, want: "/repos/:owner/:repo/issues/:issueId/events"},
		{name: "issue assignees", args: args{path: "/repos/helm/charts/issues/15756/assignees"}, want: "/repos/:owner/:repo/issues/:issueId/assignees"},
		{name: "issue reactions", args: args{path: "/repos/kubernetes-sigs/cluster-api-provider-aws/issues/958/reactions"}, want: "/repos/:owner/:repo/issues/:issueId/reactions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simplifier.Simplify(tt.args.path); got != tt.want {
				t.Errorf("GetSimplifiedPath(%s) = %v, want %v", tt.args.path, got, tt.want)
			}
		})
	}
}

func Test_GetSimplifiedPathRepositories(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "access repositories/ path should not fail, is not explicitly handled", args: args{path: "/repositories"}, want: "/repositories"},

		{name: "repo issues", args: args{path: "/repositories/testRepository/issues"}, want: "/repositories/:repoId/issues"},
		{name: "repo issue by number", args: args{path: "/repositories/testRepository/issues/21342"}, want: "/repositories/:repoId/issues/:issueId"},
		{name: "repo issue by number lock", args: args{path: "/repositories/testRepository/issues/21321/lock"}, want: "/repositories/:repoId/issues/:issueId/lock"},
		{name: "repo issues comments", args: args{path: "/repositories/testRepository/issues/comments"}, want: "/repositories/:repoId/issues/comments"},
		{name: "repo issues comment by number", args: args{path: "/repositories/testRepository/issues/comments/321"}, want: "/repositories/:repoId/issues/comments/:commentId"},
		{name: "repo issues events", args: args{path: "/repositories/testRepository/issues/events"}, want: "/repositories/:repoId/issues/events"},
		{name: "repo issues event by number", args: args{path: "/repositories/testRepository/issues/events/123"}, want: "/repositories/:repoId/issues/events/:eventId"},

		{name: "repo keys", args: args{path: "/repositories/testRepository/keys"}, want: "/repositories/:repoId/keys"},
		{name: "repo key by id", args: args{path: "/repositories/testRepository/keys/421"}, want: "/repositories/:repoId/keys/:keyId"},

		{name: "repo labels", args: args{path: "/repositories/testRepository/labels"}, want: "/repositories/:repoId/labels"},
		{name: "repo label by name", args: args{path: "/repositories/testRepository/labels/testLabel"}, want: "/repositories/:repoId/labels/:labelId"},

		{name: "repo merges", args: args{path: "/repositories/testRepository/merges"}, want: "/repositories/:repoId/merges"},

		{name: "repo milestones", args: args{path: "/repositories/testRepository/milestones"}, want: "/repositories/:repoId/milestones"},
		{name: "repo milestones by number", args: args{path: "/repositories/testRepository/milestones/421"}, want: "/repositories/:repoId/milestones/:milestone"},

		{name: "repo pulls", args: args{path: "/repositories/testRepository/pulls"}, want: "/repositories/:repoId/pulls"},
		{name: "repo pulls by number", args: args{path: "/repositories/testRepository/pulls/421"}, want: "/repositories/:repoId/pulls/:pullId"},

		{name: "repo releases", args: args{path: "/repositories/testRepository/releases"}, want: "/repositories/:repoId/releases"},
		{name: "repo releases by number", args: args{path: "/repositories/testRepository/releases/421"}, want: "/repositories/:repoId/releases/:releaseId"},

		{name: "repo stargazers", args: args{path: "/repositories/testRepository/stargazers"}, want: "/repositories/:repoId/stargazers"},

		{name: "repo statuses", args: args{path: "/repositories/testRepository/statuses"}, want: "/repositories/:repoId/statuses"},
		{name: "repo statuses by sha", args: args{path: "/repositories/testRepository/statuses/4u8dsaag89ewfdjkt9fdajdsa"}, want: "/repositories/:repoId/statuses/:statusId"},

		{name: "repo subscribers", args: args{path: "/repositories/testRepository/subscribers"}, want: "/repositories/:repoId/subscribers"},

		{name: "repo subscribers", args: args{path: "/repositories/testRepository/subscribers"}, want: "/repositories/:repoId/subscribers"},

		{name: "repo notifications", args: args{path: "/repositories/testRepository/notifications"}, want: "/repositories/:repoId/notifications"},

		{name: "repo branches", args: args{path: "/repositories/testRepository/branches"}, want: "/repositories/:repoId/branches"},
		{name: "repo branches by name", args: args{path: "/repositories/testRepository/branches/testBranch"}, want: "/repositories/:repoId/branches/:branch"},
		{name: "repo branches protection by name ", args: args{path: "/repositories/testRepository/branches/testBranch/protection"}, want: "/repositories/:repoId/branches/:branch/protection"},
		{name: "repo branches protection (required status checks) by name ", args: args{path: "/repositories/testRepository/branches/testBranch/protection/required_status_checks"}, want: "/repositories/:repoId/branches/:branch/protection/required_status_checks"},
		{name: "repo branches protection (required status checks, contexts) by name ", args: args{path: "/repositories/testRepository/branches/testBranch/protection/required_status_checks/contexts"}, want: "/repositories/:repoId/branches/:branch/protection/required_status_checks/contexts"},
		{name: "repo branches protection (required pull request reviews) by name ", args: args{path: "/repositories/testRepository/branches/testBranch/protection/required_pull_request_reviews"}, want: "/repositories/:repoId/branches/:branch/protection/required_pull_request_reviews"},
		{name: "repo branches protection (required signatures) by name ", args: args{path: "/repositories/testRepository/branches/testBranch/protection/required_signatures"}, want: "/repositories/:repoId/branches/:branch/protection/required_signatures"},
		{name: "repo branches protection (enforce admins) by name ", args: args{path: "/repositories/testRepository/branches/testBranch/protection/enforce_admins"}, want: "/repositories/:repoId/branches/:branch/protection/enforce_admins"},
		{name: "repo branches protection (restrictions) by name ", args: args{path: "/repositories/testRepository/branches/testBranch/protection/restrictions"}, want: "/repositories/:repoId/branches/:branch/protection/restrictions"},
		{name: "repo branches protection (restrictions for teams) by name ", args: args{path: "/repositories/testRepository/branches/testBranch/protection/restrictions/teams"}, want: "/repositories/:repoId/branches/:branch/protection/restrictions/teams"},
		{name: "repo branches protection (restrictions for users) by name ", args: args{path: "/repositories/testRepository/branches/testBranch/protection/restrictions/users"}, want: "/repositories/:repoId/branches/:branch/protection/restrictions/users"},

		{name: "repo archive", args: args{path: "/repositories/testRepository/archive"}, want: "/repositories/:repoId/archive"},
		{name: "repo archive ref", args: args{path: "/repositories/testRepository/archive/tar.gz"}, want: "/repositories/:repoId/archive/:zip"},

		{name: "repo assignees", args: args{path: "/repositories/testRepository/assignees"}, want: "/repositories/:repoId/assignees"},
		{name: "repo assignees by name", args: args{path: "/repositories/testRepository/assignees/testUser"}, want: "/repositories/:repoId/assignees/:assigneeId"},

		{name: "repo git commits", args: args{path: "/repositories/testRepository/git/commits"}, want: "/repositories/:repoId/git/commits"},
		{name: "repo git commit by sha", args: args{path: "/repositories/testRepository/git/commits/4u8dsaag89ewfdjkt9fdajdsa"}, want: "/repositories/:repoId/git/commits/:sha"},
		{name: "repo git refs", args: args{path: "/repositories/testRepository/git/ref"}, want: "/repositories/:repoId/git/ref"},
		{name: "repo git ref by sha", args: args{path: "/repositories/testRepository/git/ref/4u8dsaag89ewfdjkt9fdajdsa"}, want: "/repositories/:repoId/git/ref/:refId"},
		{name: "repo git tags", args: args{path: "/repositories/testRepository/git/tags"}, want: "/repositories/:repoId/git/tags"},
		{name: "repo git tag by sha", args: args{path: "/repositories/testRepository/git/tags/4u8dsaag89ewfdjkt9fdajdsa"}, want: "/repositories/:repoId/git/tags/:tagId"},
		{name: "repo git trees", args: args{path: "/repositories/testRepository/git/trees"}, want: "/repositories/:repoId/git/trees"},
		{name: "repo git tree by sha", args: args{path: "/repositories/testRepository/git/trees/4u8dsaag89ewfdjkt9fdajdsa"}, want: "/repositories/:repoId/git/trees/:sha"},

		{name: "repo git tags", args: args{path: "/repositories/testRepository/hooks"}, want: "/repositories/:repoId/hooks"},

		{name: "repo collaborators", args: args{path: "/repositories/testRepository/collaborators"}, want: "/repositories/:repoId/collaborators"},
		{name: "repo collaborators by name", args: args{path: "/repositories/testRepository/collaborators/testCollaborator"}, want: "/repositories/:repoId/collaborators/:collaboratorId"},

		{name: "repo comments", args: args{path: "/repositories/testRepository/comments"}, want: "/repositories/:repoId/comments"},
		{name: "repo comments by id", args: args{path: "/repositories/testRepository/comments/testComment"}, want: "/repositories/:repoId/comments/:commentId"},

		{name: "repo commits", args: args{path: "/repositories/testRepository/commits"}, want: "/repositories/:repoId/commits"},
		{name: "repo commits by sha", args: args{path: "/repositories/testRepository/commits/testCommitSha"}, want: "/repositories/:repoId/commits/:sha"},

		// /compare/base...head
		{name: "repo compare", args: args{path: "/repositories/testRepository/compare/testBase...testHead"}, want: "/repositories/:repoId/compare/:sha"},

		{name: "repo contents", args: args{path: "/repositories/testRepository/contents"}, want: "/repositories/:repoId/contents"},
		{name: "repo contents by name", args: args{path: "/repositories/testRepository/contents/testContents"}, want: "/repositories/:repoId/contents/:contentId"},

		{name: "repo deployments", args: args{path: "/repositories/testRepository/deployments"}, want: "/repositories/:repoId/deployments"},

		{name: "repo downloads", args: args{path: "/repositories/testRepository/downloads"}, want: "/repositories/:repoId/downloads"},

		{name: "repo events", args: args{path: "/repositories/testRepository/events"}, want: "/repositories/:repoId/events"},

		{name: "repo forks", args: args{path: "/repositories/testRepository/forks"}, want: "/repositories/:repoId/forks"},

		{name: "repo topics", args: args{path: "/repositories/testRepository/topics"}, want: "/repositories/:repoId/topics"},

		{name: "repo vulnerability-alerts", args: args{path: "/repositories/testRepository/vulnerability-alerts"}, want: "/repositories/:repoId/vulnerability-alerts"},

		{name: "repo automated-security-fixes", args: args{path: "/repositories/testRepository/automated-security-fixes"}, want: "/repositories/:repoId/automated-security-fixes"},

		{name: "repo contributors", args: args{path: "/repositories/testRepository/contributors"}, want: "/repositories/:repoId/contributors"},

		{name: "repo languages", args: args{path: "/repositories/testRepository/languages"}, want: "/repositories/:repoId/languages"},

		{name: "repo teams", args: args{path: "/repositories/testRepository/teams"}, want: "/repositories/:repoId/teams"},

		{name: "repo tags", args: args{path: "/repositories/testRepository/tags"}, want: "/repositories/:repoId/tags"},

		{name: "repo transfer", args: args{path: "/repositories/testRepository/transfer"}, want: "/repositories/:repoId/transfer"},

		{name: "master ref", args: args{path: "/repositories/168397/git/refs/heads/master"}, want: "/repositories/:repoId/git/refs/heads/:ref"},
		{name: "issue comments", args: args{path: "/repositories/168397/issues/104/comments"}, want: "/repositories/:repoId/issues/:issueId/comments"},
		{name: "issue labels", args: args{path: "/repositories/168397/issues/104/labels"}, want: "/repositories/:repoId/issues/:issueId/labels"},
		{name: "issue label", args: args{path: "/repositories/168397/issues/104/labels/needs-rebase"}, want: "/repositories/:repoId/issues/:issueId/labels/:labelId"},
		{name: "issue events", args: args{path: "/repositories/168397/issues/15756/events"}, want: "/repositories/:repoId/issues/:issueId/events"},
		{name: "issue assignees", args: args{path: "/repositories/168397/issues/15756/assignees"}, want: "/repositories/:repoId/issues/:issueId/assignees"},
		{name: "issue reactions", args: args{path: "/repositories/168397/issues/958/reactions"}, want: "/repositories/:repoId/issues/:issueId/reactions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simplifier.Simplify(tt.args.path); got != tt.want {
				t.Errorf("GetSimplifiedPath(%s) = %v, want %v", tt.args.path, got, tt.want)
			}
		})
	}
}

func Test_GetSimplifiedPathUser(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "user", args: args{path: "/user"}, want: "/user"},

		{name: "user email", args: args{path: "/user/emails"}, want: "/user/emails"},
		{name: "user email visibility", args: args{path: "/user/email/visibility"}, want: "/user/email/visibility"},

		{name: "user public emails", args: args{path: "/user/public_emails"}, want: "/user/public_emails"},

		{name: "user followers", args: args{path: "/user/followers"}, want: "/user/followers"},

		{name: "user following", args: args{path: "/user/following"}, want: "/user/following"},
		{name: "user following user", args: args{path: "/user/following/testUser"}, want: "/user/following/:userId"},

		{name: "user starred", args: args{path: "/user/starred"}, want: "/user/starred"},

		{name: "user issues", args: args{path: "/user/issues"}, want: "/user/issues"},

		{name: "user keys", args: args{path: "/user/keys"}, want: "/user/keys"},
		{name: "user keys by id", args: args{path: "/user/keys/testKey"}, want: "/user/keys/:keyId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simplifier.Simplify(tt.args.path); got != tt.want {
				t.Errorf("GetSimplifiedPath(%s) = %v, want %v", tt.args.path, got, tt.want)
			}
		})
	}
}

func Test_GetSimplifiedPathUsers(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "users", args: args{path: "/users"}, want: "/users"},
		{name: "users", args: args{path: "/users/testUser"}, want: "/users/:username"},

		{name: "users username repos", args: args{path: "/users/testUser/repos"}, want: "/users/:username/repos"},

		{name: "users username hovercard", args: args{path: "/users/testUser/hovercard"}, want: "/users/:username/hovercard"},

		{name: "users username followers", args: args{path: "/users/testUser/followers"}, want: "/users/:username/followers"},
		{name: "users username follows user", args: args{path: "/users/testUser/followers/testTargetUser"}, want: "/users/:username/followers/:username"},

		{name: "users username following", args: args{path: "/users/testUser/following"}, want: "/users/:username/following"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simplifier.Simplify(tt.args.path); got != tt.want {
				t.Errorf("GetSimplifiedPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_GetSimplifiedPathOrganizations(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "orgs", args: args{path: "/organizations"}, want: "/organizations"},
		{name: "orgs", args: args{path: "/organizations/testOrg"}, want: "/organizations/:orgId"},

		{name: "orgs orgname repos", args: args{path: "/organizations/testOrg/repos"}, want: "/organizations/:orgId/repos"},

		{name: "orgs orgname issues", args: args{path: "/organizations/testOrg/issues"}, want: "/organizations/:orgId/issues"},

		{name: "orgs orgname credential-authorizations", args: args{path: "/organizations/testOrg/credential-authorizations"}, want: "/organizations/:orgId/credential-authorizations"},
		{name: "orgs orgname credential-authorizations by id", args: args{path: "/organizations/testOrg/credential-authorizations/testId"}, want: "/organizations/:orgId/credential-authorizations/:credentialId"},

		{name: "org invitations", args: args{path: "/organizations/openshift/invitations"}, want: "/organizations/:orgId/invitations"},
		{name: "org members", args: args{path: "/organizations/openshift/members"}, want: "/organizations/:orgId/members"},
		{name: "org member", args: args{path: "/organizations/openshift/members/stevekuznetsov"}, want: "/organizations/:orgId/members/:login"},
		{name: "org teams", args: args{path: "/organizations/openshift/teams"}, want: "/organizations/:orgId/teams"},

		{name: "org members by ID", args: args{path: "/organizations/792337/members"}, want: "/organizations/:orgId/members"},
		{name: "org teams by ID", args: args{path: "/organizations/792337/teams"}, want: "/organizations/:orgId/teams"},
		{name: "org team members by ID", args: args{path: "/organizations/792337/team/792337/members"}, want: "/organizations/:orgId/team/:teamId/members"},
		{name: "org team repos by ID", args: args{path: "/organizations/792337/team/792337/repos"}, want: "/organizations/:orgId/team/:teamId/repos"},
		{name: "org repos by ID", args: args{path: "/organizations/792337/repos"}, want: "/organizations/:orgId/repos"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simplifier.Simplify(tt.args.path); got != tt.want {
				t.Errorf("GetSimplifiedPath(%s) = %v, want %v", tt.args.path, got, tt.want)
			}
		})
	}
}

func Test_GetSimplifiedPathOrgs(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "orgs", args: args{path: "/orgs"}, want: "/orgs"},
		{name: "orgs", args: args{path: "/orgs/testOrg"}, want: "/orgs/:orgname"},

		{name: "orgs orgname repos", args: args{path: "/orgs/testOrg/repos"}, want: "/orgs/:orgname/repos"},

		{name: "orgs orgname issues", args: args{path: "/orgs/testOrg/issues"}, want: "/orgs/:orgname/issues"},

		{name: "orgs orgname credential-authorizations", args: args{path: "/orgs/testOrg/credential-authorizations"}, want: "/orgs/:orgname/credential-authorizations"},
		{name: "orgs orgname credential-authorizations by id", args: args{path: "/orgs/testOrg/credential-authorizations/testId"}, want: "/orgs/:orgname/credential-authorizations/:credentialId"},

		{name: "org invitations", args: args{path: "/orgs/openshift/invitations"}, want: "/orgs/:orgname/invitations"},
		{name: "org members", args: args{path: "/orgs/openshift/members"}, want: "/orgs/:orgname/members"},
		{name: "org member", args: args{path: "/orgs/openshift/members/stevekuznetsov"}, want: "/orgs/:orgname/members/:login"},
		{name: "org teams", args: args{path: "/orgs/openshift/teams"}, want: "/orgs/:orgname/teams"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simplifier.Simplify(tt.args.path); got != tt.want {
				t.Errorf("GetSimplifiedPath(%s) = %v, want %v", tt.args.path, got, tt.want)
			}
		})
	}
}

func Test_GetSimplifiedPathNotifications(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "notifications", args: args{path: "/notifications"}, want: "/notifications"},
		{name: "notifications threads", args: args{path: "/notifications/threads"}, want: "/notifications/threads"},
		{name: "notifications thread by id", args: args{path: "/notifications/threads/testThreadId"}, want: "/notifications/threads/:threadId"},
		{name: "notifications thread by id", args: args{path: "/notifications/threads/testThreadId/subscription"}, want: "/notifications/threads/:threadId/subscription"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simplifier.Simplify(tt.args.path); got != tt.want {
				t.Errorf("GetSimplifiedPath(%s) = %v, want %v", tt.args.path, got, tt.want)
			}
		})
	}
}

func TestUserAgentWithoutVersion(t *testing.T) {
	tests := []struct {
		name, in, out string
	}{
		{
			name: "normal user agent gets split",
			in:   "hook.config-updater/v20200314-12f848798",
			out:  "hook.config-updater",
		},
		{
			name: "user agent without version does not split",
			in:   "some-custom-thing",
			out:  "some-custom-thing",
		},
		{
			name: "malformed user agent returns something sensible",
			in:   "some/custom/thing",
			out:  "some",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual, expected := userAgentWithoutVersion(test.in), test.out; actual != expected {
				t.Errorf("%s: expected %s, got %s", test.name, expected, actual)
			}
		})
	}
}
