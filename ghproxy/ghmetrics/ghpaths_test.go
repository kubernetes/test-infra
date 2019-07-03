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
		{"access root path should not fail, is not explicitly handled", args{"/"}, "/"},
		{"repo branches protection (restrictions for users) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/restrictions/users"}, "/repos/:owner/:repo/branches/:var/protection/restrictions/users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getSimplifiedPath(tt.args.path); got != tt.want {
				t.Errorf("getSimplifiedPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_handleRepos(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"access root path should not fail, is not explicitly handled", args{"/"}, "/repos"},
		{"access repos/ path should not fail, is not explicitly handled", args{"/repos"}, "/repos"},
		{"access repos/ path should not fail, is not explicitly handled", args{"/repos/testOwner/testRepo/"}, "/repos/:owner/:repo"},
		{"access repos/owner/repo/k8s path should not fail, is not explicitly handled", args{"/repos/testOwner/testRepo/k8s"}, "/repos/:owner/:repo/k8s"},

		{"repo issues", args{"/repos/testOwner/testRepo/issues"}, "/repos/:owner/:repo/issues"},
		{"repo issue by number", args{"/repos/testOwner/testRepo/issues/21342"}, "/repos/:owner/:repo/issues/:var"},
		{"repo issue by number lock", args{"/repos/testOwner/testRepo/issues/21321/lock"}, "/repos/:owner/:repo/issues/:var/lock"},
		{"repo issues comments", args{"/repos/testOwner/testRepo/issues/comments"}, "/repos/:owner/:repo/issues/comments"},
		{"repo issues comment by number", args{"/repos/testOwner/testRepo/issues/comments/321"}, "/repos/:owner/:repo/issues/comments/:var"},
		{"repo issues events", args{"/repos/testOwner/testRepo/issues/events"}, "/repos/:owner/:repo/issues/events"},
		{"repo issues event by number", args{"/repos/testOwner/testRepo/issues/events/123"}, "/repos/:owner/:repo/issues/events/:var"},

		{"repo keys", args{"/repos/testOwner/testRepo/keys"}, "/repos/:owner/:repo/keys"},
		{"repo key by id", args{"/repos/testOwner/testRepo/keys/421"}, "/repos/:owner/:repo/keys/:var"},

		{"repo labels", args{"/repos/testOwner/testRepo/labels"}, "/repos/:owner/:repo/labels"},
		{"repo label by name", args{"/repos/testOwner/testRepo/labels/testLabel"}, "/repos/:owner/:repo/labels/:var"},

		{"repo merges", args{"/repos/testOwner/testRepo/merges"}, "/repos/:owner/:repo/merges"},

		{"repo milestones", args{"/repos/testOwner/testRepo/milestones"}, "/repos/:owner/:repo/milestones"},
		{"repo milestones by number", args{"/repos/testOwner/testRepo/milestones/421"}, "/repos/:owner/:repo/milestones/:var"},

		{"repo pulls", args{"/repos/testOwner/testRepo/pulls"}, "/repos/:owner/:repo/pulls"},
		{"repo pulls by number", args{"/repos/testOwner/testRepo/pulls/421"}, "/repos/:owner/:repo/pulls/:var"},

		{"repo releases", args{"/repos/testOwner/testRepo/releases"}, "/repos/:owner/:repo/releases"},
		{"repo releases by number", args{"/repos/testOwner/testRepo/releases/421"}, "/repos/:owner/:repo/releases/:var"},

		{"repo stargazers", args{"/repos/testOwner/testRepo/stargazers"}, "/repos/:owner/:repo/stargazers"},

		{"repo statuses", args{"/repos/testOwner/testRepo/statuses"}, "/repos/:owner/:repo/statuses"},
		{"repo statuses by sha", args{"/repos/testOwner/testRepo/statuses/4u8dsaag89ewfdjkt9fdajdsa"}, "/repos/:owner/:repo/statuses/:var"},

		{"repo subscribers", args{"/repos/testOwner/testRepo/subscribers"}, "/repos/:owner/:repo/subscribers"},

		{"repo subscribers", args{"/repos/testOwner/testRepo/subscribers"}, "/repos/:owner/:repo/subscribers"},

		{"repo notifications", args{"/repos/testOwner/testRepo/notifications"}, "/repos/:owner/:repo/notifications"},

		{"repo branches", args{"/repos/testOwner/testRepo/branches"}, "/repos/:owner/:repo/branches"},
		{"repo branches by name", args{"/repos/testOwner/testRepo/branches/testBranch"}, "/repos/:owner/:repo/branches/:var"},
		{"repo branches protection by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection"}, "/repos/:owner/:repo/branches/:var/protection"},
		{"repo branches protection (required status checks) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/required_status_checks"}, "/repos/:owner/:repo/branches/:var/protection/required_status_checks"},
		{"repo branches protection (required status checks, contexts) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/required_status_checks/contexts"}, "/repos/:owner/:repo/branches/:var/protection/required_status_checks/contexts"},
		{"repo branches protection (required pull request reviews) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/required_pull_request_reviews"}, "/repos/:owner/:repo/branches/:var/protection/required_pull_request_reviews"},
		{"repo branches protection (required signatures) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/required_signatures"}, "/repos/:owner/:repo/branches/:var/protection/required_signatures"},
		{"repo branches protection (enforce admins) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/enforce_admins"}, "/repos/:owner/:repo/branches/:var/protection/enforce_admins"},
		{"repo branches protection (restrictions) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/restrictions"}, "/repos/:owner/:repo/branches/:var/protection/restrictions"},
		{"repo branches protection (restrictions for teams) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/restrictions/teams"}, "/repos/:owner/:repo/branches/:var/protection/restrictions/teams"},
		{"repo branches protection (restrictions for users) by name ", args{"/repos/testOwner/testRepo/branches/testBranch/protection/restrictions/users"}, "/repos/:owner/:repo/branches/:var/protection/restrictions/users"},

		{"repo archive", args{"/repos/testOwner/testRepo/archive"}, "/repos/:owner/:repo/archive"},
		{"repo archive ref", args{"/repos/testOwner/testRepo/archive/tar.gz"}, "/repos/:owner/:repo/archive/:var"},

		{"repo assignees", args{"/repos/testOwner/testRepo/assignees"}, "/repos/:owner/:repo/assignees"},
		{"repo assignees by name", args{"/repos/testOwner/testRepo/assignees/testUser"}, "/repos/:owner/:repo/assignees/:var"},

		{"repo git commits", args{"/repos/testOwner/testRepo/git/commits"}, "/repos/:owner/:repo/git/commits"},
		{"repo git commit by sha", args{"/repos/testOwner/testRepo/git/commits/4u8dsaag89ewfdjkt9fdajdsa"}, "/repos/:owner/:repo/git/commits/:var"},
		{"repo git refs", args{"/repos/testOwner/testRepo/git/ref"}, "/repos/:owner/:repo/git/ref"},
		{"repo git ref by sha", args{"/repos/testOwner/testRepo/git/ref/4u8dsaag89ewfdjkt9fdajdsa"}, "/repos/:owner/:repo/git/ref/:var"},
		{"repo git tags", args{"/repos/testOwner/testRepo/git/tags"}, "/repos/:owner/:repo/git/tags"},
		{"repo git tag by sha", args{"/repos/testOwner/testRepo/git/tags/4u8dsaag89ewfdjkt9fdajdsa"}, "/repos/:owner/:repo/git/tags/:var"},
		{"repo git trees", args{"/repos/testOwner/testRepo/git/trees"}, "/repos/:owner/:repo/git/trees"},
		{"repo git tree by sha", args{"/repos/testOwner/testRepo/git/trees/4u8dsaag89ewfdjkt9fdajdsa"}, "/repos/:owner/:repo/git/trees/:var"},

		{"repo git tags", args{"/repos/testOwner/testRepo/hooks"}, "/repos/:owner/:repo/hooks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handleRepos(tt.args.path); got != tt.want {
				t.Errorf("handleRepos() = %v, want %v", got, tt.want)
			}
		})
	}
}
