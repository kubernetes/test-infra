package ghmetrics

import "testing"

func Test_getSimplifiedPath(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"repo issues", args{"/repos/testOwner/testRepo/issues"}, "/repos/:owner/:repo/issues"},
		{"repo issue by number", args{"/repos/testOwner/testRepo/issues/21342"}, "/repos/:owner/:repo/issues/:number"},
		{"repo issue by number lock", args{"/repos/testOwner/testRepo/issues/21321/lock"}, "/repos/:owner/:repo/issues/:number/lock"},

		{"repo issues comments", args{"/repos/testOwner/testRepo/issues/comments"}, "/repos/:owner/:repo/issues/comments"},
		{"repo issues comment by number", args{"/repos/testOwner/testRepo/issues/comments/321"}, "/repos/:owner/:repo/issues/comments/:number"},

		{"repo issues events", args{"/repos/testOwner/testRepo/issues/events"}, "/repos/:owner/:repo/issues/events"},
		{"repo issues event by number", args{"/repos/testOwner/testRepo/issues/events/123"}, "/repos/:owner/:repo/issues/events/:number"},

		{"repo keys", args{"/repos/testOwner/testRepo/keys"}, "/repos/:owner/:repo/keys"},
		{"repo key by id", args{"/repos/testOwner/testRepo/keys/421"}, "/repos/:owner/:repo/keys/:number"},

		{"repo labels", args{"/repos/testOwner/testRepo/labels"}, "/repos/:owner/:repo/labels"},
		{"repo label by name", args{"/repos/testOwner/testRepo/labels/testLabel"}, "/repos/:owner/:repo/labels/:name"},

		{"repo merges", args{"/repos/testOwner/testRepo/merges"}, "/repos/:owner/:repo/merges"},

		{"repo milestones", args{"/repos/testOwner/testRepo/milestones"}, "/repos/:owner/:repo/milestones"},
		{"repo milestones by number", args{"/repos/testOwner/testRepo/milestones/421"}, "/repos/:owner/:repo/milestones/:number"},

		{"repo pulls", args{"/repos/testOwner/testRepo/pulls"}, "/repos/:owner/:repo/pulls"},
		{"repo pulls by number", args{"/repos/testOwner/testRepo/pulls/421"}, "/repos/:owner/:repo/pulls/:number"},

		{"repo releases", args{"/repos/testOwner/testRepo/releases"}, "/repos/:owner/:repo/releases"},
		{"repo releases by number", args{"/repos/testOwner/testRepo/releases/421"}, "/repos/:owner/:repo/releases/:number"},

		{"repo stargazers", args{"/repos/testOwner/testRepo/stargazers"}, "/repos/:owner/:repo/stargazers"},

		{"repo statuses", args{"/repos/testOwner/testRepo/statuses"}, "/repos/:owner/:repo/statuses"},
		{"repo statuses by sha", args{"/repos/testOwner/testRepo/statuses/4u8dsaag89ewfdjkt9fdajdsa"}, "/repos/:owner/:repo/statuses/:sha"},

		{"repo subscribers", args{"/repos/testOwner/testRepo/subscribers"}, "/repos/:owner/:repo/subscribers"},

		{"repo subscribers", args{"/repos/testOwner/testRepo/subscribers"}, "/repos/:owner/:repo/subscribers"},

		{"repo notifications", args{"/repos/testOwner/testRepo/notifications"}, "/repos/:owner/:repo/notifications"},

		{"repo branches", args{"/repos/testOwner/testRepo/branches"}, "/repos/:owner/:repo/branches"},
		{"repo branches by name", args{"/repos/testOwner/testRepo/branches/testBranch"}, "/repos/:owner/:repo/branches/:name"},
		{"repo branches protection by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection"}, "/repos/:owner/:repo/branches/:name/protection"},
		{"repo branches protection (required status checks) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/required_status_checks"}, "/repos/:owner/:repo/branches/:name/protection/required_status_checks"},
		{"repo branches protection (required status checks, contexts) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/required_status_checks/contexts"}, "/repos/:owner/:repo/branches/:name/protection/required_status_checks/contexts"},
		{"repo branches protection (required pull request reviews) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/required_pull_request_reviews"}, "/repos/:owner/:repo/branches/:name/protection/required_pull_request_reviews"},
		{"repo branches protection (required signatures) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/required_signatures"}, "/repos/:owner/:repo/branches/:name/protection/required_signatures"},
		{"repo branches protection (enforce admins) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/enforce_admins"}, "/repos/:owner/:repo/branches/:name/protection/enforce_admins"},
		{"repo branches protection (restrictions) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/restrictions"}, "/repos/:owner/:repo/branches/:name/protection/restrictions"},
		{"repo branches protection (restrictions for teams) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/restrictions/teams"}, "/repos/:owner/:repo/branches/:name/protection/restrictions/teams"},
		{"repo branches protection (restrictions for users) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/restrictions/users"}, "/repos/:owner/:repo/branches/:name/protection/restrictions/users"},

		{"repo archive", args{"/repos/testOwner/testRepo/testArchive"}, "/repos/:owner/:repo/:name"},
		{"repo archive ref", args{"/repos/testOwner/testRepo/testArchive/testRef"}, "/repos/:owner/:repo/:name/:sha"},

		{"repo assignees", args{"/repos/testOwner/testRepo/assignees"}, "/repos/:owner/:repo/assignees"},
		{"repo assignees by name", args{"/repos/testOwner/testRepo/assignees/testUser"}, "/repos/:owner/:repo/assignees/:name"},

		{"repo git commits", args{"/repos/testOwner/testRepo/git/commits"}, "/repos/:owner/:repo/git/commits"},
		{"repo git commit by sha", args{"/repos/testOwner/testRepo/git/commits/4u8dsaag89ewfdjkt9fdajdsa"}, "/repos/:owner/:repo/git/commits/:sha"},
		{"repo git refs", args{"/repos/testOwner/testRepo/git/ref"}, "/repos/:owner/:repo/git/ref"},
		{"repo git ref by sha", args{"/repos/testOwner/testRepo/git/ref/4u8dsaag89ewfdjkt9fdajdsa"}, "/repos/:owner/:repo/git/ref/:sha"},
		{"repo git tags", args{"/repos/testOwner/testRepo/git/tags"}, "/repos/:owner/:repo/git/tags"},
		{"repo git tag by sha", args{"/repos/testOwner/testRepo/git/tags/4u8dsaag89ewfdjkt9fdajdsa"}, "/repos/:owner/:repo/git/tags/:sha"},
		{"repo git trees", args{"/repos/testOwner/testRepo/git/trees"}, "/repos/:owner/:repo/git/trees"},
		{"repo git tree by sha", args{"/repos/testOwner/testRepo/git/trees/4u8dsaag89ewfdjkt9fdajdsa"}, "/repos/:owner/:repo/git/trees/:sha"},

		{"repo git tags", args{"/repos/testOwner/testRepo/hooks"}, "/repos/:owner/:repo/hooks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getSimplifiedPath(tt.args.path); got != tt.want {
				t.Errorf("getSimplifiedPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

/*

	/repos/:owner/:repo/git/commits
	/repos/:owner/:repo/git/commits/:sha
	/repos/:owner/:repo/git/refs
	/repos/:owner/:repo/git/refs/:sha
	/repos/:owner/:repo/git/tags
	/repos/:owner/:repo/git/tags/:sha
	/repos/:owner/:repo/git/blobs
	/repos/:owner/:repo/git/blobs/:sha
	/repos/:owner/:repo/git/trees
	/repos/:owner/:repo/git/tress/:sha

	/repos/:owner/:repo/hooks

	/repos/:owner/:repo/collaborators
	/repos/:owner/:repo/collaborators/:collaborator

	/repos/:owner/:repo/comments
	/repos/:owner/:repo/comments/:number

	/repos/:owner/:repo/commits
	/repos/:owner/:repo/commits/:sha

	/repos/:owner/:repo/compare/:base...:head

	/repos/:owner/:repo/contents
	/repos/:owner/:repo/contents/:path

	/repos/:owner/:repo/deployments

	/repos/:owner/:repo/downloads

	/repos/:owner/:repo/events

	/repos/:owner/:repo/forks

	/repos/:owner/:repo/topics

	/repos/:owner/:repo/vulnerability-alerts

	/repos/:owner/:repo/automated-security-fixes

	/repos/:owner/:repo/contributors

	/repos/:owner/:repo/languages

	/repos/:owner/:repo/teams

	/repos/:owner/:repo/tags

	/repos/:owner/:repo/transfer

	/repositories
*/
