/*
Copyright 2016 The Kubernetes Authors.

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

package label

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

const (
	orgMember    = "Alice"
	nonOrgMember = "Bob"
)

func formatWithPRInfo(labels ...string) []string {
	r := []string{}
	for _, l := range labels {
		r = append(r, fmt.Sprintf("%s/%s#%d:%s", "org", "repo", 1, l))
	}
	if len(r) == 0 {
		return nil
	}
	return r
}

func TestHandleComment(t *testing.T) {
	type testCase struct {
		name                  string
		body                  string
		commenter             string
		extraLabels           []string
		restrictedLabels      map[string][]plugins.RestrictedLabel
		expectedNewLabels     []string
		expectedRemovedLabels []string
		expectedBotComment    bool
		repoLabels            []string
		issueLabels           []string
		expectedCommentText   string
		action                github.GenericCommentEventAction
		teams                 map[string]map[string]fakegithub.TeamWithMembers
	}
	testcases := []testCase{
		{
			name:                  "Irrelevant comment",
			body:                  "irrelelvant",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			repoLabels:            []string{},
			issueLabels:           []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Empty Area",
			body:                  "/area",
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{"area/infra"},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Single Area Label",
			body:                  "/area infra",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Single Area Label when already present on Issue",
			body:                  "/area infra",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Single Priority Label",
			body:                  "/priority critical",
			repoLabels:            []string{"area/infra", "priority/critical"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("priority/critical"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Single Kind Label",
			body:                  "/kind bug",
			repoLabels:            []string{"area/infra", "priority/critical", labels.Bug},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo(labels.Bug),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Single Triage Label",
			body:                  "/triage needs-information",
			repoLabels:            []string{"area/infra", "triage/needs-information"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     formatWithPRInfo("triage/needs-information"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Org member can add triage/accepted label",
			body:                  "/triage accepted",
			repoLabels:            []string{"triage/accepted"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("triage/accepted"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Non org member cannot add triage/accepted label",
			body:                  "/triage accepted",
			repoLabels:            []string{"triage/accepted", "kind/bug"},
			issueLabels:           []string{"kind/bug"},
			expectedNewLabels:     formatWithPRInfo(),
			expectedRemovedLabels: []string{},
			commenter:             nonOrgMember,
			expectedBotComment:    true,
			expectedCommentText:   "The label `triage/accepted` cannot be applied. Only GitHub organization members can add the label.",
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Non org member can add triage/needs-information label",
			body:                  "/triage needs-information",
			repoLabels:            []string{"area/infra", "triage/needs-information"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     formatWithPRInfo("triage/needs-information"),
			expectedRemovedLabels: []string{},
			commenter:             nonOrgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Adding Labels is Case Insensitive",
			body:                  "/kind BuG",
			repoLabels:            []string{"area/infra", "priority/critical", labels.Bug},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo(labels.Bug),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Adding Labels is Case Insensitive",
			body:                  "/kind bug",
			repoLabels:            []string{"area/infra", "priority/critical", labels.Bug},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo(labels.Bug),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Can't Add Non Existent Label",
			body:                  "/priority critical",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo(),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    true,
			expectedCommentText:   "The label(s) `priority/critical` cannot be applied, because the repository doesn't have them.",
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Non Org Member Can't Add",
			body:                  "/area infra",
			repoLabels:            []string{"area/infra", "priority/critical", labels.Bug},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             nonOrgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Command must start at the beginning of the line",
			body:                  "  /area infra",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical", "priority/urgent", "priority/important", labels.Bug},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo(),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Can't Add Labels Non Existing Labels",
			body:                  "/area lgtm",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo(),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    true,
			expectedCommentText:   "The label(s) `area/lgtm` cannot be applied, because the repository doesn't have them.",
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Multiple Area Labels",
			body:                  "/area api infra",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("area/api", "area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Multiple Area Labels one already present on Issue",
			body:                  "/area api infra",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			issueLabels:           []string{"area/api"},
			expectedNewLabels:     formatWithPRInfo("area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Multiple Priority Labels",
			body:                  "/priority critical important",
			repoLabels:            []string{"priority/critical", "priority/important"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("priority/critical", "priority/important"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Multiple Area Labels, With Trailing Whitespace",
			body:                  "/area api infra ",
			repoLabels:            []string{"area/infra", "area/api"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("area/api", "area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Label Prefix Must Match Command (Area-Priority Mismatch)",
			body:                  "/area urgent",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo(),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    true,
			expectedCommentText:   "The label(s) `area/urgent` cannot be applied, because the repository doesn't have them.",
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Label Prefix Must Match Command (Priority-Area Mismatch)",
			body:                  "/priority infra",
			repoLabels:            []string{"area/infra", "area/api", "priority/critical", "priority/urgent"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo(),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    true,
			expectedCommentText:   "The label(s) `priority/infra` cannot be applied, because the repository doesn't have them.",
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Multiple Area Labels (Some Valid)",
			body:                  "/area lgtm infra",
			repoLabels:            []string{"area/infra", "area/api"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    true,
			expectedCommentText:   "The label(s) `area/lgtm` cannot be applied, because the repository doesn't have them.",
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Multiple Committee Labels (Some Valid)",
			body:                  "/committee steering calamity",
			repoLabels:            []string{"committee/conduct", "committee/steering"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("committee/steering"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    true,
			expectedCommentText:   "The label(s) `committee/calamity` cannot be applied, because the repository doesn't have them.",
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Non org member adds multiple triage labels (some valid)",
			body:                  "/triage needs-information accepted",
			repoLabels:            []string{"triage/needs-information", "triage/accepted"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("triage/needs-information"),
			expectedRemovedLabels: []string{},
			commenter:             nonOrgMember,
			expectedBotComment:    true,
			expectedCommentText:   "The label `triage/accepted` cannot be applied. Only GitHub organization members can add the label.",
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add Multiple Types of Labels Different Lines",
			body:                  "/priority urgent\n/area infra",
			repoLabels:            []string{"area/infra", "priority/urgent"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("priority/urgent", "area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove Area Label when no such Label on Repo",
			body:                  "/remove-area infra",
			repoLabels:            []string{},
			issueLabels:           []string{},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    true,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove Area Label when no such Label on Issue",
			body:                  "/remove-area infra",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    true,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove Area Label",
			body:                  "/remove-area infra",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatWithPRInfo("area/infra"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove Committee Label",
			body:                  "/remove-committee infinite-monkeys",
			repoLabels:            []string{"area/infra", "sig/testing", "committee/infinite-monkeys"},
			issueLabels:           []string{"area/infra", "sig/testing", "committee/infinite-monkeys"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatWithPRInfo("committee/infinite-monkeys"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove Kind Label",
			body:                  "/remove-kind api-server",
			repoLabels:            []string{"area/infra", "priority/high", "kind/api-server", "needs-kind"},
			issueLabels:           []string{"area/infra", "priority/high", "kind/api-server"},
			expectedNewLabels:     formatWithPRInfo("needs-kind"),
			expectedRemovedLabels: formatWithPRInfo("kind/api-server"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove Priority Label",
			body:                  "/remove-priority high",
			repoLabels:            []string{"area/infra", "priority/high", "needs-priority"},
			issueLabels:           []string{"area/infra", "priority/high"},
			expectedNewLabels:     formatWithPRInfo("needs-priority"),
			expectedRemovedLabels: formatWithPRInfo("priority/high"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove SIG Label",
			body:                  "/remove-sig testing",
			repoLabels:            []string{"area/infra", "sig/testing", "needs-sig"},
			issueLabels:           []string{"area/infra", "sig/testing"},
			expectedNewLabels:     formatWithPRInfo("needs-sig"),
			expectedRemovedLabels: formatWithPRInfo("sig/testing"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove one of many SIG Label",
			body:                  "/remove-sig testing",
			repoLabels:            []string{"area/infra", "sig/testing", "sig/node", "sig/auth", "needs-sig"},
			issueLabels:           []string{"area/infra", "sig/testing", "sig/node", "sig/auth"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatWithPRInfo("sig/testing"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add and Remove SIG Label",
			body:                  "/remove-sig testing\n/sig node",
			repoLabels:            []string{"area/infra", "sig/testing", "sig/node", "needs-sig"},
			issueLabels:           []string{"area/infra", "sig/testing"},
			expectedNewLabels:     formatWithPRInfo("sig/node"),
			expectedRemovedLabels: formatWithPRInfo("sig/testing"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove WG Policy",
			body:                  "/remove-wg policy",
			repoLabels:            []string{"area/infra", "wg/policy"},
			issueLabels:           []string{"area/infra", "wg/policy"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatWithPRInfo("wg/policy"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove Triage Label",
			body:                  "/remove-triage needs-information accepted",
			repoLabels:            []string{"area/infra", "triage/needs-information", "triage/accepted", "needs-triage"},
			issueLabels:           []string{"area/infra", "triage/needs-information", "triage/accepted"},
			expectedNewLabels:     formatWithPRInfo("needs-triage"),
			expectedRemovedLabels: formatWithPRInfo("triage/needs-information", "triage/accepted"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove Multiple Labels",
			body:                  "/remove-priority low high\n/remove-kind api-server\n/remove-area  infra",
			repoLabels:            []string{"area/infra", "priority/high", "priority/low", "kind/api-server"},
			issueLabels:           []string{"area/infra", "priority/high", "priority/low", "kind/api-server"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatWithPRInfo("priority/low", "priority/high", "kind/api-server", "area/infra"),
			commenter:             orgMember,
			expectedBotComment:    true,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add and Remove Label at the same time",
			body:                  "/remove-area infra\n/area test",
			repoLabels:            []string{"area/infra", "area/test"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     formatWithPRInfo("area/test"),
			expectedRemovedLabels: formatWithPRInfo("area/infra"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add and Remove the same Label",
			body:                  "/remove-area infra\n/area infra",
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{"area/infra"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatWithPRInfo("area/infra"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Multiple Add and Delete Labels",
			body:                  "/remove-area ruby\n/remove-kind srv\n/remove-priority l m\n/area go\n/kind cli\n/priority h",
			repoLabels:            []string{"area/go", "area/ruby", "kind/cli", "kind/srv", "priority/h", "priority/m", "priority/l"},
			issueLabels:           []string{"area/ruby", "kind/srv", "priority/l", "priority/m"},
			expectedNewLabels:     formatWithPRInfo("area/go", "kind/cli", "priority/h"),
			expectedRemovedLabels: formatWithPRInfo("area/ruby", "kind/srv", "priority/l", "priority/m"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Do nothing with empty /label command",
			body:                  "/label",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Do nothing with empty /remove-label command",
			body:                  "/remove-label",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add custom label",
			body:                  "/label orchestrator/foo",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("orchestrator/foo"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add custom label with trailing space",
			body:                  "/label orchestrator/foo ",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("orchestrator/foo"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add custom label with trailing LF newline",
			body:                  "/label orchestrator/foo\n",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("orchestrator/foo"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Add custom label with trailing CRLF newline",
			body:                  "/label orchestrator/foo\r\n",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("orchestrator/foo"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Cannot add missing custom label",
			body:                  "/label orchestrator/foo",
			extraLabels:           []string{"orchestrator/jar", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    true,
			expectedCommentText:   "The label(s) `/label orchestrator/foo` cannot be applied. These labels are supported: `orchestrator/jar, orchestrator/bar`",
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove custom label",
			body:                  "/remove-label orchestrator/foo",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{"orchestrator/foo"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatWithPRInfo("orchestrator/foo"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove custom label with trailing space",
			body:                  "/remove-label orchestrator/foo ",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{"orchestrator/foo"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatWithPRInfo("orchestrator/foo"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove custom label with trailing LF newline",
			body:                  "/remove-label orchestrator/foo\n",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{"orchestrator/foo"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatWithPRInfo("orchestrator/foo"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Remove custom label with trailing CRLF newline",
			body:                  "/remove-label orchestrator/foo\r\n",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{"orchestrator/foo"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: formatWithPRInfo("orchestrator/foo"),
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Cannot remove missing custom label",
			body:                  "/remove-label orchestrator/jar",
			extraLabels:           []string{"orchestrator/foo", "orchestrator/bar"},
			repoLabels:            []string{"orchestrator/foo"},
			issueLabels:           []string{"orchestrator/foo"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    true,
			expectedCommentText:   "The label(s) `/remove-label orchestrator/jar` cannot be applied. These labels are supported: `orchestrator/foo, orchestrator/bar`",
			action:                github.GenericCommentActionCreated,
		},
		{
			name:                  "Don't comment when deleting label addition",
			body:                  "/kind bug",
			repoLabels:            []string{"area/infra", "priority/critical", labels.Bug},
			issueLabels:           []string{},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    false,
			action:                github.GenericCommentActionDeleted,
		},
		{
			name:                  "Don't comment when deleting label removal",
			body:                  "/remove-committee infinite-monkeys",
			repoLabels:            []string{"area/infra", "sig/testing", "committee/infinite-monkeys"},
			issueLabels:           []string{"area/infra", "sig/testing", "committee/infinite-monkeys"},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    false,
			action:                github.GenericCommentActionDeleted,
		},
		{
			name:                  "Don't take action while editing body",
			body:                  "/kind bug",
			repoLabels:            []string{labels.Bug},
			issueLabels:           []string{},
			expectedNewLabels:     []string{},
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			expectedBotComment:    false,
			action:                github.GenericCommentActionEdited,
		},
		{
			name: "Strip markdown comments",
			body: `
<!--
/kind bug
/kind cleanup
-->
/area infra
`,
			repoLabels:            []string{"area/infra"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("area/infra"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name: "Strip markdown comments non greedy",
			body: `
<!--
/kind bug
-->
/kind cleanup
<!--
/area infra
-->
/kind regression
`,
			repoLabels:            []string{"kind/cleanup", "kind/regression"},
			issueLabels:           []string{},
			expectedNewLabels:     formatWithPRInfo("kind/cleanup", "kind/regression"),
			expectedRemovedLabels: []string{},
			commenter:             orgMember,
			action:                github.GenericCommentActionCreated,
		},
		{
			name:               "Restricted label addition, user is not in group",
			body:               `/label restricted-label`,
			repoLabels:         []string{"restricted-label"},
			commenter:          orgMember,
			restrictedLabels:   map[string][]plugins.RestrictedLabel{"org": {{Label: "restricted-label", AllowedTeams: []string{"privileged-group"}}}},
			action:             github.GenericCommentActionCreated,
			expectedBotComment: true,
		},
		{
			name:              "Restricted label addition, user is in group",
			body:              `/label restricted-label`,
			repoLabels:        []string{"restricted-label"},
			commenter:         orgMember,
			restrictedLabels:  map[string][]plugins.RestrictedLabel{"org": {{Label: "restricted-label", AllowedTeams: []string{"privileged-group"}}}},
			action:            github.GenericCommentActionCreated,
			teams:             map[string]map[string]fakegithub.TeamWithMembers{"org": {"privileged-group": {Members: sets.NewString(orgMember)}}},
			expectedNewLabels: formatWithPRInfo("restricted-label"),
		},
		{
			name:              "Restricted label addition, user is in allowed_users",
			body:              `/label restricted-label`,
			repoLabels:        []string{"restricted-label"},
			commenter:         orgMember,
			restrictedLabels:  map[string][]plugins.RestrictedLabel{"org": {{Label: "restricted-label", AllowedUsers: []string{orgMember}}}},
			action:            github.GenericCommentActionCreated,
			expectedNewLabels: formatWithPRInfo("restricted-label"),
		},
		{
			name:               "Restricted label removal, user is not in group",
			body:               `/remove-label restricted-label`,
			repoLabels:         []string{"restricted-label"},
			issueLabels:        []string{"restricted-label"},
			commenter:          orgMember,
			restrictedLabels:   map[string][]plugins.RestrictedLabel{"org": {{Label: "restricted-label", AllowedTeams: []string{"privileged-group"}}}},
			action:             github.GenericCommentActionCreated,
			expectedBotComment: true,
		},
		{
			name:                  "Restricted label removal, user is in group",
			body:                  `/remove-label restricted-label`,
			repoLabels:            []string{"restricted-label"},
			issueLabels:           []string{"restricted-label"},
			commenter:             orgMember,
			restrictedLabels:      map[string][]plugins.RestrictedLabel{"org": {{Label: "restricted-label", AllowedTeams: []string{"privileged-group"}}}},
			action:                github.GenericCommentActionCreated,
			teams:                 map[string]map[string]fakegithub.TeamWithMembers{"org": {"privileged-group": {Members: sets.NewString(orgMember)}}},
			expectedRemovedLabels: formatWithPRInfo("restricted-label"),
		},
		{
			name:                  "Restricted label removal, user is in allowed_users",
			body:                  `/remove-label restricted-label`,
			repoLabels:            []string{"restricted-label"},
			issueLabels:           []string{"restricted-label"},
			commenter:             orgMember,
			restrictedLabels:      map[string][]plugins.RestrictedLabel{"org": {{Label: "restricted-label", AllowedUsers: []string{orgMember}}}},
			action:                github.GenericCommentActionCreated,
			expectedRemovedLabels: formatWithPRInfo("restricted-label"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			sort.Strings(tc.expectedNewLabels)
			fakeClient := fakegithub.NewFakeClient()
			fakeClient.Issues = make(map[int]*github.Issue)
			fakeClient.IssueComments = make(map[int][]github.IssueComment)
			fakeClient.RepoLabelsExisting = tc.repoLabels
			fakeClient.OrgMembers = map[string][]string{"org": {orgMember}}
			fakeClient.IssueLabelsAdded = []string{}
			fakeClient.IssueLabelsRemoved = []string{}
			fakeClient.Teams = tc.teams
			// Add initial labels
			for _, label := range tc.issueLabels {
				fakeClient.AddLabel("org", "repo", 1, label)
			}
			e := &github.GenericCommentEvent{
				Action: tc.action,
				Body:   tc.body,
				Number: 1,
				Repo:   github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				User:   github.User{Login: tc.commenter},
			}
			err := handleComment(fakeClient, logrus.WithField("plugin", PluginName), plugins.Label{AdditionalLabels: tc.extraLabels, RestrictedLabels: tc.restrictedLabels}, e)
			if err != nil {
				t.Fatalf("didn't expect error from handle comment test: %v", err)
			}

			// Check that all the correct labels (and only the correct labels) were added.
			expectLabels := append(formatWithPRInfo(tc.issueLabels...), tc.expectedNewLabels...)
			if expectLabels == nil {
				expectLabels = []string{}
			}
			sort.Strings(expectLabels)
			sort.Strings(fakeClient.IssueLabelsAdded)
			if diff := cmp.Diff(expectLabels, fakeClient.IssueLabelsAdded, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("labels expected to add do not match actual added labels: %s", diff)
			}

			sort.Strings(tc.expectedRemovedLabels)
			sort.Strings(fakeClient.IssueLabelsRemoved)
			if diff := cmp.Diff(tc.expectedRemovedLabels, fakeClient.IssueLabelsRemoved, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("expected removed labels differ from actual removed labels: %s", diff)
			}
			if len(fakeClient.IssueCommentsAdded) > 0 && !tc.expectedBotComment {
				t.Errorf("unexpected bot comments: %#v", fakeClient.IssueCommentsAdded)
			}
			if len(fakeClient.IssueCommentsAdded) == 0 && tc.expectedBotComment {
				t.Error("expected a bot comment but got none")
			}
			if tc.expectedBotComment && len(tc.expectedCommentText) > 0 {
				if len(fakeClient.IssueComments) < 1 {
					t.Errorf("expected actual: %v", fakeClient.IssueComments)
				}
				if len(fakeClient.IssueComments[1]) != 1 || !strings.Contains(fakeClient.IssueComments[1][0].Body, tc.expectedCommentText) {
					t.Errorf("expected: `%v`, actual: `%v`", tc.expectedCommentText, fakeClient.IssueComments[1][0].Body)
				}
			}
		})
	}
}

func TestHandleLabelAdd(t *testing.T) {
	type testCase struct {
		name              string
		restrictedLabels  map[string][]plugins.RestrictedLabel
		expectedAssignees []string
		labelAdded        string
		action            github.PullRequestEventAction
	}
	testCases := []testCase{
		{
			name:       "label added with no auto-assign configured",
			labelAdded: "some-label",
			action:     github.PullRequestActionLabeled,
		},
		{
			name:              "assign users for restricted label on label add",
			restrictedLabels:  map[string][]plugins.RestrictedLabel{"org": {{Label: "secondary-label", AllowedUsers: []string{"bill", "sally"}, AssignOn: []plugins.AssignOnLabel{{Label: "initial-label"}}}}},
			labelAdded:        "initial-label",
			action:            github.PullRequestActionLabeled,
			expectedAssignees: formatWithPRInfo("bill", "sally"),
		},
		{
			name:             "no assigned users on irrelevant label add",
			restrictedLabels: map[string][]plugins.RestrictedLabel{"org": {{Label: "secondary-label", AllowedUsers: []string{"bill", "sally"}, AssignOn: []plugins.AssignOnLabel{{Label: "initial-label"}}}}},
			labelAdded:       "other-label",
			action:           github.PullRequestActionLabeled,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fakegithub.NewFakeClient()
			e := &github.PullRequestEvent{
				Action:      tc.action,
				Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				PullRequest: github.PullRequest{Number: 1},
				Label:       github.Label{Name: tc.labelAdded},
			}
			err := handleLabelAdd(fakeClient, logrus.WithField("plugin", PluginName), plugins.Label{RestrictedLabels: tc.restrictedLabels}, e)
			if err != nil {
				t.Fatalf("didn't expect error from handle label test: %v", err)
			}
			if diff := cmp.Diff(tc.expectedAssignees, fakeClient.AssigneesAdded, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("expected added assignees differ from actual: %s", diff)
			}
		})
	}
}

func TestHelpProvider(t *testing.T) {
	enabledRepos := []config.OrgRepo{
		{Org: "org1", Repo: "repo"},
		{Org: "org2", Repo: "repo"},
	}
	cases := []struct {
		name               string
		config             *plugins.Configuration
		enabledRepos       []config.OrgRepo
		err                bool
		configInfoIncludes []string
	}{
		{
			name:               "Empty config",
			config:             &plugins.Configuration{},
			enabledRepos:       enabledRepos,
			configInfoIncludes: []string{configString(defaultLabels)},
		},
		{
			name: "With AdditionalLabels",
			config: &plugins.Configuration{
				Label: plugins.Label{
					AdditionalLabels: []string{"sig", "triage", "wg"},
				},
			},
			enabledRepos:       enabledRepos,
			configInfoIncludes: []string{configString(append(defaultLabels, "sig", "triage", "wg"))},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pluginHelp, err := helpProvider(c.config, c.enabledRepos)
			if err != nil && !c.err {
				t.Fatalf("helpProvider error: %v", err)
			}
			for _, msg := range c.configInfoIncludes {
				if !strings.Contains(pluginHelp.Config[""], msg) {
					t.Fatalf("helpProvider.Config error mismatch: didn't get %v, but wanted it", msg)
				}
			}
		})
	}
}
