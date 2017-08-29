/*
Copyright 2017 The Kubernetes Authors.

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
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungers/approvers"
	c "k8s.io/test-infra/mungegithub/mungers/matchers/comment"
	"k8s.io/test-infra/mungegithub/mungers/matchers/event"
	"k8s.io/test-infra/mungegithub/mungers/mungerutil"
	"k8s.io/test-infra/mungegithub/options"

	githubapi "github.com/google/go-github/github"
)

const (
	milestoneNotifierName = "MilestoneNotifier"

	milestoneRemoved          = "Milestone Removed"
	milestoneLabelsIncomplete = "Milestone Labels Incomplete"
	milestoneLabelsComplete   = "Milestone Labels Complete"

	priorityCriticalUrgent = "priority/critical-urgent"

	commentDetail = `<details>
Additional instructions available [here](https://github.com/kubernetes/community/blob/master/contributors/devel/release/issues.md)
</details>`
)

var (
	kindMap = map[string]string{
		"kind/bug":     "Fixes a bug discovered during the current release.",
		"kind/feature": "New functionality.",
		"kind/cleanup": "Adding tests, refactoring, fixing old bugs.",
	}

	priorityMap = map[string]string{
		priorityCriticalUrgent:        "Never automatically move out of a release milestone; continually escalate to contributor and SIG through all available channels.",
		"priority/important-soon":     "Escalate to the issue owners and SIG owner; move out of milestone after several unsuccessful escalation attempts.",
		"priority/important-longterm": "Escalate to the issue owners; move out of the milestone after 1 attempt.",
	}

	milestoneRemovedLabel          = strings.ToLower(strings.Replace(milestoneRemoved, " ", "-", -1))
	milestoneLabelsIncompleteLabel = strings.ToLower(strings.Replace(milestoneLabelsIncomplete, " ", "-", -1))
	milestoneLabelsCompleteLabel   = strings.ToLower(strings.Replace(milestoneLabelsComplete, " ", "-", -1))
	milestoneLabels                = []string{milestoneRemovedLabel, milestoneLabelsCompleteLabel, milestoneLabelsIncompleteLabel}
)

// milestoneNotification configures the active notification for the munger
type milestoneNotification struct {
	description string
	message     string
}

// MilestoneMaintainer enforces the process for sheperding issues into the release.
type MilestoneMaintainer struct {
	botName  string
	features *features.Features

	activeMilestone string
	gracePeriod     time.Duration
	warningInterval time.Duration
}

func init() {
	RegisterMungerOrDie(&MilestoneMaintainer{})
}

// Name is the name usable in --pr-mungers
func (m *MilestoneMaintainer) Name() string { return "milestone-maintainer" }

// RequiredFeatures is a slice of 'features' that must be provided
func (m *MilestoneMaintainer) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (m *MilestoneMaintainer) Initialize(config *github.Config, features *features.Features) error {
	if len(m.activeMilestone) == 0 {
		return errors.New("active-milestone must be supplied")
	}
	if m.gracePeriod <= 0 {
		return errors.New("milestone-grace-period must be greater than zero")
	}
	if m.warningInterval <= 0 {
		return errors.New("milestone-warning-interval must be greater than zero")
	}
	m.botName = config.BotName
	m.features = features
	return nil
}

// EachLoop is called at the start of every munge loop.  This function
// is a no-op for the munger because to munge an issue it only needs
// the state local to the issue.
func (m *MilestoneMaintainer) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (m *MilestoneMaintainer) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterString(&m.activeMilestone, "active-milestone", "", "The active milestone that this munger will maintain issues for.")
	opts.RegisterDuration(&m.gracePeriod, "milestone-grace-period", 72*time.Hour, "The grace period to wait before removing an incomplete issue from the active milestone.")
	opts.RegisterDuration(&m.warningInterval, "milestone-warning-interval", 24*time.Hour, "The interval to wait between warning about an incomplete issue in the active milestone.")
	opts.RegisterUpdateCallback(func(changed sets.String) error {
		if changed.Has("active-milestone") {
			if len(m.activeMilestone) == 0 {
				return errors.New("active-milestone must be supplied")
			}
		}
		if changed.Has("milestone-grace-period") {
			if m.gracePeriod <= 0 {
				return errors.New("milestone-grace-period must be greater than zero")
			}
		}
		if changed.Has("milestone-warning-interval") {
			if m.warningInterval <= 0 {
				return errors.New("milestone-warning-interval must be greater than zero")
			}
		}
		return nil
	})
	return nil
}

// Munge is the workhorse the will actually make updates to the issue
func (m *MilestoneMaintainer) Munge(obj *github.MungeObject) {
	if ignoreObject(obj, m.activeMilestone) {
		return
	}

	comment, ok := latestNotificationComment(obj, m.botName)
	if !ok {
		return
	}

	labelName, notifyState := notificationState(obj, comment, m.botName, m.gracePeriod, m.warningInterval)
	// Always attempt to ensure the milestone label
	if len(labelName) > 0 && !ensureLabel(obj, labelName) {
		return
	}
	if notifyState == nil {
		// No notification change is required
		return
	}

	if comment != nil {
		if err := obj.DeleteComment(comment.Source.(*githubapi.IssueComment)); err != nil {
			return
		}
	}

	notification := c.Notification{
		Name:      milestoneNotifierName,
		Arguments: notifyState.description,
		Context:   notifyState.message,
	}
	if err := notification.Post(obj); err != nil {
		return
	}

	if labelName == milestoneRemovedLabel {
		obj.ClearMilestone()
	}
}

// ignoreObject indicates whether the munger should ignore the given
// object.  Only issues in the active milestone should be munged.
func ignoreObject(obj *github.MungeObject, activeMilestone string) bool {
	// Only target issues
	if obj.IsPR() {
		return true
	}

	// Only target issues with an assigned milestone
	milestone, ok := obj.ReleaseMilestone()
	if !ok || len(milestone) == 0 {
		return true
	}

	// Only target issues in the active milestone
	return milestone != activeMilestone
}

// notificationState returns the state required to ensure the munger's
// notification is kept current.  If a nil return value is returned,
// no action should be taken.
func notificationState(obj *github.MungeObject, comment *c.Comment, botName string, gracePeriod, warningInterval time.Duration) (string, *milestoneNotification) {
	notification := c.ParseNotification(comment)

	kindLabel, priorityLabel, sigLabels, labelErrors := checkLabels(obj.Issue.Labels)

	labelsComplete := len(labelErrors) == 0
	if labelsComplete {
		return milestoneLabelsCompleteLabel, labelsCompleteState(obj.Issue, notification, kindLabel, priorityLabel, sigLabels)
	}

	isBlocker := (priorityLabel == priorityCriticalUrgent)
	var removeAfter *time.Duration
	// Blockers are never removed from the milestone so grace period computation can be skipped
	if !isBlocker {
		removeAfter = gracePeriodRemaining(obj, botName, gracePeriod, time.Now())
		failedToComputeGracePeriod := removeAfter == nil
		if failedToComputeGracePeriod {
			return "", nil
		}
	}
	// removeAfter is guaranteed to be non-nil for non-blockers
	if isBlocker || *removeAfter >= 0 {
		var createdAt *time.Time
		// createdAt should be nil for blockers to avoid repeatedly posting the same warning message
		if !isBlocker && comment != nil {
			createdAt = comment.CreatedAt
		}
		return milestoneLabelsIncompleteLabel, labelsIncompleteState(obj.Issue, notification, labelErrors, warningInterval, createdAt, removeAfter)
	}
	return milestoneRemovedLabel, removalState(obj.Issue, notification, labelErrors, *obj.Issue.Milestone.Title, gracePeriod)
}

// latestNotificationComment returns the most recent notification
// comment posted by the munger.
//
// Since the munger is careful to remove existing comments before
// adding new ones, only a single notification comment should exist.
func latestNotificationComment(obj *github.MungeObject, botName string) (*c.Comment, bool) {
	issueComments, ok := obj.ListComments()
	if !ok {
		return nil, false
	}
	comments := c.FromIssueComments(issueComments)
	notificationMatcher := c.MungerNotificationName(milestoneNotifierName, botName)
	notifications := c.FilterComments(comments, notificationMatcher)
	return notifications.GetLast(), true
}

// labelsCompleteState returns the notification state for an issue
// that has the required labels to remain in the active milestone.
func labelsCompleteState(issue *githubapi.Issue, notification *c.Notification, kindLabel, priorityLabel string, sigLabels []string) *milestoneNotification {
	const template = `{{.mention}}

Issue label settings:

- {{range $index, $sigLabel := .sigLabels}}{{if $index}} {{end}}{{$sigLabel}}{{end}}: Issue will be escalated to these SIGs if needed.
- {{.priorityLabel}}: {{.priorityDescription}}
- {{.kindLabel}}: {{.kindDescription}}

{{.detail}}
`

	isCurrent := notification != nil && notification.Arguments == milestoneLabelsComplete
	if isCurrent {
		return nil
	}

	mention := mungerutil.GetIssueUsers(issue).AllUsers().Mention().Join()
	message := approvers.GenerateTemplateOrFail(template, "message", map[string]interface{}{
		"mention":             mention,
		"sigLabels":           sigLabels,
		"priorityLabel":       priorityLabel,
		"priorityDescription": priorityMap[priorityLabel],
		"kindLabel":           kindLabel,
		"kindDescription":     kindMap[kindLabel],
		"detail":              commentDetail,
	})
	if message == nil {
		return nil
	}

	return &milestoneNotification{
		description: milestoneLabelsComplete,
		message:     *message,
	}
}

// labelsIncompleteState returns the notification state for an issue
// lacking the labels required for the active milestone.
func labelsIncompleteState(issue *githubapi.Issue, notification *c.Notification, labelErrors []string, warningInterval time.Duration, commentCreatedAt *time.Time, removeAfter *time.Duration) *milestoneNotification {
	const template = `{{.mention}}

**Action required**: This issue requires label changes.{{.warning}}

{{range $index, $labelError := .labelErrors -}}
{{$labelError}}
{{end}}
{{.detail}}
`

	isCurrent := (notification != nil && notification.Arguments == milestoneLabelsIncomplete && (commentCreatedAt == nil || time.Since(*commentCreatedAt) < warningInterval))
	if isCurrent {
		return nil
	}

	mention := mungerutil.GetIssueUsers(issue).AllUsers().Mention().Join()
	var warning string
	if removeAfter != nil {
		warning = fmt.Sprintf("  If the required changes are not made within %s, the issue will be moved out of the %s milestone.", durationToMaxDays(*removeAfter), *issue.Milestone.Title)
	}
	message := approvers.GenerateTemplateOrFail(template, "message", map[string]interface{}{
		"mention":     mention,
		"warning":     warning,
		"labelErrors": labelErrors,
		"detail":      commentDetail,
	})
	if message == nil {
		return nil
	}

	return &milestoneNotification{
		description: milestoneLabelsIncomplete,
		message:     *message,
	}
}

// removalState returns the notification state for an issue that will
// be removed from the active milestone.
func removalState(issue *githubapi.Issue, notification *c.Notification, labelErrors []string, milestone string, gracePeriod time.Duration) *milestoneNotification {
	const template = `{{.mention}}

**Important**:
This issue was missing labels required for the {{.milestone}} milestone for more than {{.dayPhrase}}:

{{range $index, $labelError := .labelErrors -}}
{{$labelError}}
{{end}}
Removing it from the milestone.

{{.detail}}
`

	mention := mungerutil.GetIssueUsers(issue).AllUsers().Mention().Join()
	message := approvers.GenerateTemplateOrFail(template, "message", map[string]interface{}{
		"mention":     mention,
		"milestone":   milestone,
		"dayPhrase":   durationToMaxDays(gracePeriod),
		"labelErrors": labelErrors,
		"detail":      commentDetail,
	})
	if message == nil {
		return nil
	}

	return &milestoneNotification{
		description: milestoneRemoved,
		message:     *message,
	}
}

// ensureLabel ensures that the desired label becomes the only
// milestone label set on the given issue.  Returns true if the label
// is set on the issue, false otherwise.  Any error encountered is
// expected to be logged in the called function rather than being
// handled by the caller.
func ensureLabel(obj *github.MungeObject, desiredLabel string) bool {
	for _, label := range milestoneLabels {
		if label == desiredLabel {
			if !obj.HasLabel(label) {
				if err := obj.AddLabel(label); err != nil {
					return false
				}
			}
		} else if obj.HasLabel(label) {
			if err := obj.RemoveLabel(label); err != nil {
				return false
			}
		}
	}
	return true
}

// gracePeriodRemaining returns the difference between the start of
// the grace period and the grace period interval.  Returns nil the
// grace period start cannot be determined.
func gracePeriodRemaining(obj *github.MungeObject, botName string, gracePeriod time.Duration, defaultStart time.Time) *time.Duration {
	tempStart := gracePeriodStart(obj, botName, defaultStart)
	if tempStart == nil {
		return nil
	}
	start := *tempStart

	remaining := -time.Since(start.Add(gracePeriod))
	return &remaining
}

// gracePeriodStart determines when the grace period for the given
// object should start as is indicated by when the
// milestone-labels-incomplete label was last applied.  If the label
// is not set, the default will be returned.  nil will be returned if
// an error occurs while accessing the object's label events.
func gracePeriodStart(obj *github.MungeObject, botName string, defaultStart time.Time) *time.Time {
	labelName := milestoneLabelsIncompleteLabel
	if !obj.HasLabel(labelName) {
		return &defaultStart
	}

	return labelLastCreatedAt(obj, botName, labelName)
}

// labelLastCreatedAt returns the time at which the given label was
// last applied to the given github object.  Returns nil if an error
// occurs during event retrieval or if the label has never been set.
func labelLastCreatedAt(obj *github.MungeObject, botName, labelName string) *time.Time {
	events, ok := obj.GetEvents()
	if !ok {
		return nil
	}

	labelMatcher := event.And([]event.Matcher{
		event.AddLabel{},
		event.LabelName(labelName),
		event.Actor(botName),
	})
	labelEvents := event.FilterEvents(events, labelMatcher)
	lastAdded := labelEvents.GetLast()
	if lastAdded != nil {
		return lastAdded.CreatedAt
	}
	return nil
}

// checkLabels validates that the given labels are consistent with the
// requirements for an issue remaining in its chosen milestone.
// Returns the values of required labels (if present) and a slice of
// errors (where labels are not correct).
func checkLabels(labels []githubapi.Label) (kindLabel, priorityLabel string, sigLabels []string, labelErrors []string) {
	labelErrors = []string{}
	var err error

	kindLabel, err = uniqueLabelName(labels, kindMap)
	if err != nil || len(kindLabel) == 0 {
		labelErrors = append(labelErrors, "kind: Must specify at most one of ['kind/bug', 'kind/feature', 'kind/cleanup'].")
	}

	priorityLabel, err = uniqueLabelName(labels, priorityMap)
	if err != nil || len(priorityLabel) == 0 {
		labelErrors = append(labelErrors, "priority: Must specify at most one of ['priority/critical-urgent', 'priority/important-soon', 'priority/important-longterm'].")
	}

	sigLabels = sigLabelNames(labels)
	if len(sigLabels) == 0 {
		labelErrors = append(labelErrors, "sig owner: Must specify at least one label prefixed with 'sig/'.")
	}

	return
}

// uniqueLabelName determines which label of a set indicated by a map
// - if any - is present in the given slice of labels.  Returns an
// error if the slice contains more than one label from the set.
func uniqueLabelName(labels []githubapi.Label, labelMap map[string]string) (string, error) {
	var labelName string
	for _, label := range labels {
		_, exists := labelMap[*label.Name]
		if exists {
			if len(labelName) == 0 {
				labelName = *label.Name
			} else {
				return "", errors.New("Found more than one matching label")
			}
		}
	}
	return labelName, nil
}

// sigLabelNames returns a slice of the 'sig/' prefixed labels set on the issue.
func sigLabelNames(labels []githubapi.Label) []string {
	labelNames := []string{}
	for _, label := range labels {
		if strings.HasPrefix(*label.Name, "sig/") {
			labelNames = append(labelNames, *label.Name)
		}
	}
	return labelNames
}
