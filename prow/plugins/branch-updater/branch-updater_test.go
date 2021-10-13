package branch_updater

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
)

const (
	dummySHA     = "deadbeef"
	dummyPrTitle = "PR title"

	gitHubPrMergeableState_Clean  = github.MergeableState("clean")
	gitHubPrMergeableState_Dirty  = github.MergeableState("dirty")
	gitHubPrMergeableState_Behind = github.MergeableState("behind")

	gitHubStatusContext_Success = "success"
	gitHubStatusContext_Failed  = "failed"
	gitHubStatusContext_Pending = "pending"
)

// Use *bool to replicate GitHub's behavior, which is treat this as a tri-state bool (true/false/nil).
var (
	mergeable    = func() *bool { b := true; return &b }()
	notMergeable = func() *bool { b := false; return &b }()
)

func TestBranchUpdater(t *testing.T) {
	for _, tc := range []struct {
		name           string
		statusContexts map[string]*github.CombinedStatus
		prStatus       github.PullRequest
		wantErr        bool
	}{
		{
			name:           "Tide-mergeable and GitHub-mergeable",
			statusContexts: makeStatusContextsWithTestData(true, true),
			prStatus:       makeGitHubPullRequest(mergeable, gitHubPrMergeableState_Clean),
			wantErr:        false,
		},
		{
			name:           "Tide-mergeable, not GitHub-mergeable",
			statusContexts: makeStatusContextsWithTestData(true, true),
			prStatus:       makeGitHubPullRequest(notMergeable, gitHubPrMergeableState_Dirty),
			wantErr:        false,
		},
		{
			name:           "Not Tide-mergeable, GitHub-mergeable",
			statusContexts: makeStatusContextsWithTestData(true, false),
			prStatus:       makeGitHubPullRequest(mergeable, gitHubPrMergeableState_Clean),
			wantErr:        false,
		},
		{
			name:           "Non-mergeable PR",
			statusContexts: makeStatusContextsWithTestData(true, false),
			prStatus:       makeGitHubPullRequest(notMergeable, gitHubPrMergeableState_Dirty),
			wantErr:        false,
		},
		{
			name:           "Repo not Tide-enabled (no Tide context)",
			statusContexts: makeStatusContextsWithTestData(false, false),
			prStatus:       makeGitHubPullRequest(mergeable, gitHubPrMergeableState_Clean),
			wantErr:        false,
		},
		{
			name:           "GitHub mergeable_state is not 'behind'",
			statusContexts: makeStatusContextsWithTestData(true, true),
			prStatus:       makeGitHubPullRequest(notMergeable, gitHubPrMergeableState_Dirty),
			wantErr:        false,
		},
		{
			name:           "PR branch requires update",
			statusContexts: makeStatusContextsWithTestData(true, true),
			prStatus:       makeGitHubPullRequest(notMergeable, gitHubPrMergeableState_Behind),
			wantErr:        false,
		},
		{
			name:           "PR branch update fails",
			statusContexts: makeStatusContextsWithTestData(true, true),
			prStatus:       makeGitHubPullRequest(notMergeable, gitHubPrMergeableState_Behind),
			wantErr:        false,
		},
	} {
		fmt.Printf("Testing case %q\n", tc.name)
		fc := &FakeUpdaterClient{
			FakeClient: fakegithub.FakeClient{
				CombinedStatuses: tc.statusContexts,
			},
		}

		// This configuration tells the plugin it's enabled for all repos.
		cfg := plugins.BranchUpdater{
			DefaultEnabled: true,
			IgnoredRepos:   []string{},
			IncludeRepos:   []string{},
		}
		e := &github.PullRequestEvent{
			Action:      github.PullRequestActionOpened,
			PullRequest: tc.prStatus,
			Number:      1,
		}

		err := handlePR(fc, logrus.WithField("plugin", pluginName), cfg, e)
		if tc.wantErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}
}

// The existing FakeClient doesn't have a fake method for UpdatePullRequestBranch so we make our own.
type FakeUpdaterClient struct {
	fakegithub.FakeClient
}

// Doesn't need to do anything, just exist and not throw an error.
func (f *FakeUpdaterClient) UpdatePullRequestBranch(org string, repo string, number int, expectedHeadSha *string) error {
	return nil
}

func makeStatusContextsWithTestData(tideEnabled bool, tideSuccess bool) map[string]*github.CombinedStatus {
	statuses := []github.Status{
		{
			State:   gitHubStatusContext_Success,
			Context: "test1",
		},
		{
			State:   gitHubStatusContext_Failed,
			Context: "test2",
		},
		{
			State:   gitHubStatusContext_Pending,
			Context: "test3",
		},
	}

	// Only include the Tide status if it's enabled
	if tideEnabled {
		var state string
		if tideSuccess {
			state = gitHubStatusContext_Success
		} else {
			state = gitHubStatusContext_Failed
		}

		statuses = append(statuses, github.Status{
			State:   state,
			Context: gitHubContextNameTide,
		})
	}

	ctxs := make(map[string]*github.CombinedStatus, 1)

	ctxs[dummySHA] = &github.CombinedStatus{
		SHA:      dummySHA,
		Statuses: statuses,
		State:    gitHubStatusContext_Failed,
	}

	return ctxs
}

func makeGitHubPullRequest(isMergeable *bool, mergeableState github.MergeableState) github.PullRequest {
	return github.PullRequest{
		ID:     1,
		Number: 1,
		Head: github.PullRequestBranch{
			Ref: dummySHA,
			SHA: dummySHA,
		},
		Mergable:       isMergeable,
		MergeableState: mergeableState,
		Title:          dummyPrTitle,
	}
}
