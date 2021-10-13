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

import (
	"k8s.io/test-infra/prow/simplifypath"
)

func repositoryTree() []simplifypath.Node {
	return []simplifypath.Node{
		l("branches", v("branch", l("protection",
			l("restrictions", l("users"), l("teams")),
			l("required_status_checks", l("contexts")),
			l("required_pull_request_reviews"),
			l("required_signatures"),
			l("enforce_admins")))),
		l("issues",
			l("comments", v("commentId")),
			l("events", v("eventId")),
			v("issueId",
				l("lock"),
				l("comments"),
				l("events"),
				l("assignees"),
				l("reactions"),
				l("labels", simplifypath.VGreedy("labelId")))),
		l("keys", v("keyId")),
		l("labels", v("labelId")),
		l("milestones", v("milestone")),
		l("pulls",
			v("pullId",
				l("commits"),
				l("files"),
				l("comments"),
				l("reviews"),
				l("requested_reviewers"),
				l("merge"))),
		l("releases", v("releaseId")),
		l("statuses", v("statusId")),
		l("subscribers", v("subscriberId")),
		l("assignees", v("assigneeId")),
		l("archive", v("zip")),
		l("collaborators", v("collaboratorId", l("permission"))),
		l("comments", v("commentId")),
		l("compare", v("sha")),
		l("contents", v("contentId")),
		l("commits",
			v("sha",
				l("check-runs"),
				l("status")),
		),
		l("git",
			l("commits", v("sha")),
			l("ref", v("refId")),
			l("tags", v("tagId")),
			l("trees", v("sha")),
			l("refs", l("heads", v("ref")))),
		l("stars"),
		l("merges"),
		l("stargazers"),
		l("notifications"),
		l("hooks"),
		l("deployments"),
		l("downloads"),
		l("events"),
		l("forks"),
		l("topics"),
		l("vulnerability-alerts"),
		l("automated-security-fixes"),
		l("contributors"),
		l("languages"),
		l("teams"),
		l("tags"),
		l("transfer"),
	}
}

func organizationTree() []simplifypath.Node {
	return []simplifypath.Node{
		l("credential-authorizations", v("credentialId")),
		l("repos"),
		l("issues"),
		l("invitations"),
		l("members", v("login")),
		l("memberships", v("login")),
		l("teams"),
		l("team", v("teamId",
			l("repos"),
			l("members"))),
	}
}

var simplifier = simplifypath.NewSimplifier(l("", // shadow element mimicing the root
	l(""),
	l("app", l("installations", v("id", l("access_tokens")))),
	l("repos",
		v("owner",
			v("repo",
				repositoryTree()...))),
	l("repositories",
		v("repoId",
			repositoryTree()...)),
	l("user",
		l("following", v("userId")),
		l("keys", v("keyId")),
		l("email", l("visibility")),
		l("emails"),
		l("public_emails"),
		l("followers"),
		l("starred"),
		l("issues"),
		v("id", l("repos")),
	),
	l("users",
		v("username",
			l("followers", v("username")),
			l("repos"),
			l("hovercard"),
			l("following"))),
	l("orgs",
		v("orgname",
			organizationTree()...)),
	l("organizations",
		v("orgId",
			organizationTree()...)),
	l("organizations",
		v("orgId",
			l("members"),
			l("repos"),
			l("teams"))),
	l("issues", v("issueId")),
	l("search",
		l("repositories"),
		l("commits"),
		l("code"),
		l("issues"),
		l("users"),
		l("topics"),
		l("labels")),
	l("gists",
		l("public"),
		l("starred")),
	l("notifications", l("threads", v("threadId", l("subscription")))),
	l("emojis"),
	l("events"),
	l("feeds"),
	l("hub"),
	l("rate_limit"),
	l("teams", v("id",
		l("members"),
		l("memberships", v("user")),
		l("repos", v("org", v("repo"))),
		l("invitations"),
	)),
	// end point for gh api v4
	l("graphql"),
	l("licenses")))

// l and v keep the tree legible

func l(fragment string, children ...simplifypath.Node) simplifypath.Node {
	return simplifypath.L(fragment, children...)
}

func v(fragment string, children ...simplifypath.Node) simplifypath.Node {
	return simplifypath.V(fragment, children...)
}
