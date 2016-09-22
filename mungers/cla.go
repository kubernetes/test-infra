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

package mungers

import (
	"time"

	"k8s.io/contrib/mungegithub/features"
	githubhelper "k8s.io/contrib/mungegithub/github"
	c "k8s.io/contrib/mungegithub/mungers/matchers/comment"
	"k8s.io/contrib/mungegithub/mungers/mungerutil"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	cncfclaNotFoundMessage = `Thanks for your pull request.  It looks like this may be your first contribution to a CNCF open source project. Before we can look at your pull request, you'll need to sign a Contributor License Agreement (CLA).

:memo: **Please visit <https://identity.linuxfoundation.org/projects/cncf> to sign.**

Once you've signed, please reply here (e.g. "I signed it!") and we'll verify.  Thanks.

---

- If you've already signed a CLA, it's possible we don't have your GitHub username or you're using a different email address.  Check your existing CLA data and verify that your [email is set on your git commits](https://help.github.com/articles/setting-your-email-in-git/).
- If you signed the CLA as a corporation, please sign in with your organization's credentials at <https://identity.linuxfoundation.org/projects/cncf> to be authorized.

<!-- need_sender_cla -->
	`

	claSignedMessage = `CLAs look good, thanks!`
	contextPending   = "pending"
	contextSuccess   = "success"
	contextError     = "error"
	contextFailure   = "failure"
	claNagNotifyName = "CLA-PING"

	// timePeriod is the time between successive posts of the CLA nag notification.
	timePeriod = 10 * 24 * time.Hour
	maxPings   = 3
)

// ClaMunger will check the CLA status of the PR and apply a label.
type ClaMunger struct {
	CLAStatusContext string
	pinger           *c.Pinger
}

var _ Munger = &ClaMunger{}

func init() {
	RegisterMungerOrDie(&ClaMunger{})
}

// Name is the name usable in --pr-mungers
func (cla *ClaMunger) Name() string { return "cla" }

// RequiredFeatures is a slice of 'features' that must be provided.
func (cla *ClaMunger) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger.
func (cla *ClaMunger) Initialize(config *githubhelper.Config, features *features.Features) error {
	if len(cla.CLAStatusContext) == 0 {
		glog.Fatalf("No --cla-status-context flag set with cla munger.")
	}

	cla.pinger = c.NewPinger(claNagNotifyName).
		SetDescription(cncfclaNotFoundMessage).SetTimePeriod(timePeriod).SetMaxCount(maxPings)

	return nil
}

// EachLoop is called at the start of every munge loop
func (cla *ClaMunger) EachLoop() error {
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`.
func (cla *ClaMunger) AddFlags(cmd *cobra.Command, config *githubhelper.Config) {
	cmd.Flags().StringVar(&cla.CLAStatusContext, "cla-status-context", "", "Status context to check to find if CLA is signed.")
}

// Munge is unused by this munger.
func (cla *ClaMunger) Munge(obj *githubhelper.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if obj.HasLabel(claHumanLabel) {
		return
	}

	status := obj.GetStatusState([]string{cla.CLAStatusContext})

	// Check for pending status and exit.
	if status == contextPending {
		// do nothing and wait for state to be updated.
		return
	}

	if status == contextSuccess {
		if obj.HasLabel(cncfClaYesLabel) {
			// status is success and we've already applied 'cncf-cla: yes'.
			return
		}
		if obj.HasLabel(cncfClaNoLabel) {
			obj.RemoveLabel(cncfClaNoLabel)
		}
		obj.AddLabel(cncfClaYesLabel)
		return
	}

	// If we are here, that means that the context is failure/error.
	comments, err := obj.ListComments()
	if err != nil {
		glog.Error(err)
		return
	}
	who := mungerutil.GetIssueUsers(obj.Issue).Author.Mention().Join()

	// Get a notification if it's time to ping.
	notif := cla.pinger.PingNotification(
		comments,
		who,
		nil,
	)
	if notif != nil {
		obj.WriteComment(notif.String())
	}

	if obj.HasLabel(cncfClaNoLabel) {
		// status reported error/failure and we've already applied 'cncf-cla: no' label.
		return
	}

	if obj.HasLabel(cncfClaYesLabel) {
		obj.RemoveLabel(cncfClaYesLabel)
	}
	obj.AddLabel(cncfClaNoLabel)
}
