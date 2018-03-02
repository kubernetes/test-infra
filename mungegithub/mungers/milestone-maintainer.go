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
	"math"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungers/approvers"
	c "k8s.io/test-infra/mungegithub/mungers/matchers/comment"
	"k8s.io/test-infra/mungegithub/mungers/matchers/event"
	"k8s.io/test-infra/mungegithub/mungers/mungerutil"
	"k8s.io/test-infra/mungegithub/options"

	githubapi "github.com/google/go-github/github"
)

type milestoneState int

type milestoneOptName string

// milestoneStateConfig defines the label and notification
// configuration for a given milestone state.
type milestoneStateConfig struct {
	// The milestone label to apply to the label (all other milestone state labels will be removed)
	label string
	// The title of the notification message
	title string
	// Whether the notification should be repeated on the configured interval
	warnOnInterval bool
	// Whether sigs should be mentioned in the notification message
	notifySIGs bool
}

const (
	day                   = time.Hour * 24
	milestoneNotifierName = "MilestoneNotifier"

	milestoneModeDev    = "dev"
	milestoneModeSlush  = "slush"
	milestoneModeFreeze = "freeze"

	milestoneCurrent        milestoneState = iota // No change is required.
	milestoneNeedsLabeling                        // One or more priority/*, kind/* and sig/* labels are missing.
	milestoneNeedsApproval                        // The status/needs-approval label is missing.
	milestoneNeedsAttention                       // A status/* label is missing or an update is required.
	milestoneNeedsRemoval                         // The issue needs to be removed from the milestone.

	milestoneLabelsIncompleteLabel = "milestone/incomplete-labels"
	milestoneNeedsApprovalLabel    = "milestone/needs-approval"
	milestoneNeedsAttentionLabel   = "milestone/needs-attention"
	milestoneRemovedLabel          = "milestone/removed"

	statusApprovedLabel   = "status/approved-for-milestone"
	statusInProgressLabel = "status/in-progress"

	blockerLabel = "priority/critical-urgent"

	sigLabelPrefix     = "sig/"
	sigMentionTemplate = "@kubernetes/sig-%s-misc"

	milestoneOptModes                = "milestone-modes"
	milestoneOptWarningInterval      = "milestone-warning-interval"
	milestoneOptLabelGracePeriod     = "milestone-label-grace-period"
	milestoneOptApprovalGracePeriod  = "milestone-approval-grace-period"
	milestoneOptSlushUpdateInterval  = "milestone-slush-update-interval"
	milestoneOptFreezeUpdateInterval = "milestone-freeze-update-interval"
	milestoneOptFreezeDate           = "milestone-freeze-date"

	milestoneDetail = `<details>
<summary>Help</summary>
<ul>
 <li><a href="https://github.com/kubernetes/community/blob/master/contributors/devel/release/issues.md">Additional instructions</a></li>
 <li><a href="https://go.k8s.io/bot-commands">Commands for setting labels</a></li>
</ul>
</details>
`

	milestoneMessageTemplate = `
{{- if .warnUnapproved}}
**Action required**: This {{.objType}} must have the {{.approvedLabel}} label applied by a SIG maintainer.{{.unapprovedRemovalWarning}}
{{end -}}
{{- if .removeUnapproved}}
**Important**: This {{.objType}} was missing the {{.approvedLabel}} label for more than {{.approvalGracePeriod}}.
{{end -}}
{{- if .warnMissingInProgress}}
**Action required**: During code {{.mode}}, {{.objTypePlural}} in the milestone should be in progress.
If this {{.objType}} is not being actively worked on, please remove it from the milestone.
If it is being worked on, please add the {{.inProgressLabel}} label so it can be tracked with other in-flight {{.objTypePlural}}.
{{end -}}
{{- if .warnUpdateRequired}}
**Action Required**: This {{.objType}} has not been updated since {{.lastUpdated}}. Please provide an update.
{{end -}}
{{- if .warnUpdateInterval}}
**Note**: This {{.objType}} is marked as {{.blockerLabel}}, and must be updated every {{.updateInterval}} during code {{.mode}}.

Example update:

` + "```" + `
ACK.  In progress
ETA: DD/MM/YYYY
Risks: Complicated fix required
` + "```" + `
{{end -}}
{{- if .warnNonBlockerRemoval}}
**Note**: If this {{.objType}} is not resolved or labeled as {{.blockerLabel}} by {{.freezeDate}} it will be moved out of the {{.milestone}}.
{{end -}}
{{- if .removeNonBlocker}}
**Important**: Code freeze is in effect and only {{.objTypePlural}} with {{.blockerLabel}} may remain in the {{.milestone}}.
{{end -}}
{{- if .warnIncompleteLabels}}
**Action required**: This {{.objType}} requires label changes.{{.incompleteLabelsRemovalWarning}}

{{range $index, $labelError := .labelErrors -}}
{{$labelError}}
{{end -}}
{{end -}}
{{- if .removeIncompleteLabels}}
**Important**: This {{.objType}} was missing labels required for the {{.milestone}} for more than {{.labelGracePeriod}}:

{{range $index, $labelError := .labelErrors -}}
{{$labelError}}
{{end}}
{{end -}}
{{- if .summarizeLabels -}}
<details{{if .onlySummary}} open{{end}}>
<summary>{{.objTypeTitle}} Labels</summary>

- {{range $index, $sigLabel := .sigLabels}}{{if $index}} {{end}}{{$sigLabel}}{{end}}: {{.objTypeTitle}} will be escalated to these SIGs if needed.
- {{.priorityLabel}}: {{.priorityDescription}}
- {{.kindLabel}}: {{.kindDescription}}
</details>
{{- end -}}
`
)

var (
	milestoneModes = sets.NewString(milestoneModeDev, milestoneModeSlush, milestoneModeFreeze)

	milestoneStateConfigs = map[milestoneState]milestoneStateConfig{
		milestoneCurrent: {
			title: "Milestone %s: **Up-to-date for process**",
		},
		milestoneNeedsLabeling: {
			title:          "Milestone %s Labels **Incomplete**",
			label:          milestoneLabelsIncompleteLabel,
			warnOnInterval: true,
		},
		milestoneNeedsApproval: {
			title:          "Milestone %s **Needs Approval**",
			label:          milestoneNeedsApprovalLabel,
			warnOnInterval: true,
			notifySIGs:     true,
		},
		milestoneNeedsAttention: {
			title:          "Milestone %s **Needs Attention**",
			label:          milestoneNeedsAttentionLabel,
			warnOnInterval: true,
			notifySIGs:     true,
		},
		milestoneNeedsRemoval: {
			title:      "Milestone **Removed** From %s",
			label:      milestoneRemovedLabel,
			notifySIGs: true,
		},
	}

	// milestoneStateLabels is the set of milestone labels applied by
	// the munger.  statusApprovedLabel is not included because it is
	// applied manually rather than by the munger.
	milestoneStateLabels = []string{
		milestoneLabelsIncompleteLabel,
		milestoneNeedsApprovalLabel,
		milestoneNeedsAttentionLabel,
		milestoneRemovedLabel,
	}

	kindMap = map[string]string{
		"kind/bug":     "Fixes a bug discovered during the current release.",
		"kind/feature": "New functionality.",
		"kind/cleanup": "Adding tests, refactoring, fixing old bugs.",
	}

	priorityMap = map[string]string{
		blockerLabel:                  "Never automatically move %s out of a release milestone; continually escalate to contributor and SIG through all available channels.",
		"priority/important-soon":     "Escalate to the %s owners and SIG owner; move out of milestone after several unsuccessful escalation attempts.",
		"priority/important-longterm": "Escalate to the %s owners; move out of the milestone after 1 attempt.",
	}
)

// issueChange encapsulates changes to make to an issue.
type issueChange struct {
	notification        *c.Notification
	label               string
	commentInterval     *time.Duration
	removeFromMilestone bool
}

type milestoneArgValidator func(name string) error

// MilestoneMaintainer enforces the process for shepherding issues into the release.
type MilestoneMaintainer struct {
	botName    string
	features   *features.Features
	validators map[string]milestoneArgValidator

	milestoneModes       string
	milestoneModeMap     map[string]string
	warningInterval      time.Duration
	labelGracePeriod     time.Duration
	approvalGracePeriod  time.Duration
	slushUpdateInterval  time.Duration
	freezeUpdateInterval time.Duration
	freezeDate           string
}

func init() {
	RegisterMungerOrDie(NewMilestoneMaintainer())
}

func NewMilestoneMaintainer() *MilestoneMaintainer {
	m := &MilestoneMaintainer{}
	m.validators = map[string]milestoneArgValidator{
		milestoneOptModes: func(name string) error {
			modeMap, err := parseMilestoneModes(m.milestoneModes)
			if err != nil {
				return fmt.Errorf("%s: %s", name, err)
			}
			m.milestoneModeMap = modeMap
			return nil
		},
		milestoneOptWarningInterval: func(name string) error {
			return durationGreaterThanZero(name, m.warningInterval)
		},
		milestoneOptLabelGracePeriod: func(name string) error {
			return durationGreaterThanZero(name, m.labelGracePeriod)
		},
		milestoneOptApprovalGracePeriod: func(name string) error {
			return durationGreaterThanZero(name, m.approvalGracePeriod)
		},
		milestoneOptSlushUpdateInterval: func(name string) error {
			return durationGreaterThanZero(name, m.slushUpdateInterval)
		},
		milestoneOptFreezeUpdateInterval: func(name string) error {
			return durationGreaterThanZero(name, m.freezeUpdateInterval)
		},
		milestoneOptFreezeDate: func(name string) error {
			if len(m.freezeDate) == 0 {
				return fmt.Errorf("%s must be supplied", name)
			}
			return nil
		},
	}
	return m
}
func durationGreaterThanZero(name string, value time.Duration) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than zero", name)
	}
	return nil
}

func dayPhrase(days int) string {
	dayString := "days"
	if days == 1 || days == -1 {
		dayString = "day"
	}
	return fmt.Sprintf("%d %s", days, dayString)
}

func durationToMaxDays(duration time.Duration) string {
	days := int(math.Ceil(duration.Hours() / 24))
	return dayPhrase(days)
}

func findLastHumanPullRequestUpdate(obj *github.MungeObject) (*time.Time, bool) {
	pr, ok := obj.GetPR()
	if !ok {
		return nil, ok
	}

	comments, ok := obj.ListReviewComments()
	if !ok {
		return nil, ok
	}

	lastHuman := pr.CreatedAt
	for i := range comments {
		comment := comments[i]
		if comment.User == nil || comment.User.Login == nil || comment.CreatedAt == nil || comment.Body == nil {
			continue
		}
		if obj.IsRobot(comment.User) || *comment.User.Login == jenkinsBotName {
			continue
		}
		if lastHuman.Before(*comment.UpdatedAt) {
			lastHuman = comment.UpdatedAt
		}
	}

	return lastHuman, true
}

func findLastHumanIssueUpdate(obj *github.MungeObject) (*time.Time, bool) {
	lastHuman := obj.Issue.CreatedAt

	comments, ok := obj.ListComments()
	if !ok {
		return nil, ok
	}

	for i := range comments {
		comment := comments[i]
		if !validComment(comment) {
			continue
		}
		if obj.IsRobot(comment.User) || jenkinsBotComment(comment) {
			continue
		}
		if lastHuman.Before(*comment.UpdatedAt) {
			lastHuman = comment.UpdatedAt
		}
	}

	return lastHuman, true
}

func findLastInterestingEventUpdate(obj *github.MungeObject) (*time.Time, bool) {
	lastInteresting := obj.Issue.CreatedAt

	events, ok := obj.GetEvents()
	if !ok {
		return nil, ok
	}

	for i := range events {
		event := events[i]
		if event.Event == nil || *event.Event != "reopened" {
			continue
		}

		if lastInteresting.Before(*event.CreatedAt) {
			lastInteresting = event.CreatedAt
		}
	}

	return lastInteresting, true
}

func findLastModificationTime(obj *github.MungeObject) (*time.Time, bool) {
	lastHumanIssue, ok := findLastHumanIssueUpdate(obj)
	if !ok {
		return nil, ok
	}

	lastInterestingEvent, ok := findLastInterestingEventUpdate(obj)
	if !ok {
		return nil, ok
	}

	var lastModif *time.Time
	lastModif = lastHumanIssue

	if lastInterestingEvent.After(*lastModif) {
		lastModif = lastInterestingEvent
	}

	if obj.IsPR() {
		lastHumanPR, ok := findLastHumanPullRequestUpdate(obj)
		if !ok {
			return lastModif, true
		}

		if lastHumanPR.After(*lastModif) {
			lastModif = lastHumanPR
		}
	}

	return lastModif, true
}

// parseMilestoneModes transforms a string containing milestones and
// their modes to a map:
//
//     "v1.8=dev,v1.9=slush" -> map[string][string]{"v1.8": "dev", "v1.9": "slush"}
func parseMilestoneModes(target string) (map[string]string, error) {
	const invalidFormatTemplate = "expected format for each milestone is [milestone]=[mode], got '%s'"

	result := map[string]string{}
	tokens := strings.Split(target, ",")
	for _, token := range tokens {
		parts := strings.Split(token, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf(invalidFormatTemplate, token)
		}
		milestone := strings.TrimSpace(parts[0])
		mode := strings.TrimSpace(parts[1])
		if len(milestone) == 0 || len(mode) == 0 {
			return nil, fmt.Errorf(invalidFormatTemplate, token)
		}
		if !milestoneModes.Has(mode) {
			return nil, fmt.Errorf("mode for milestone '%s' must be one of %v, but got '%s'", milestone, milestoneModes.List(), mode)
		}
		if _, exists := result[milestone]; exists {
			return nil, fmt.Errorf("milestone %s is specified more than once", milestone)
		}
		result[milestone] = mode
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one milestone must be configured")
	}

	return result, nil
}

// Name is the name usable in --pr-mungers
func (m *MilestoneMaintainer) Name() string { return "milestone-maintainer" }

// RequiredFeatures is a slice of 'features' that must be provided
func (m *MilestoneMaintainer) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (m *MilestoneMaintainer) Initialize(config *github.Config, features *features.Features) error {
	for name, validator := range m.validators {
		if err := validator(name); err != nil {
			return err
		}
	}

	m.botName = config.BotName
	m.features = features
	return nil
}

// EachLoop is called at the start of every munge loop. This function
// is a no-op for the munger because to munge an issue it only needs
// the state local to the issue.
func (m *MilestoneMaintainer) EachLoop() error { return nil }

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (m *MilestoneMaintainer) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterString(&m.milestoneModes, milestoneOptModes, "", fmt.Sprintf("The comma-separated list of milestones and the mode to maintain them in (one of %v). Example: v1.8=%s,v1.9=%s", milestoneModes.List(), milestoneModeDev, milestoneModeSlush))
	opts.RegisterDuration(&m.warningInterval, milestoneOptWarningInterval, 24*time.Hour, "The interval to wait between warning about an incomplete issue/pr in the active milestone.")
	opts.RegisterDuration(&m.labelGracePeriod, milestoneOptLabelGracePeriod, 72*time.Hour, "The grace period to wait before removing a non-blocking issue/pr with incomplete labels from the active milestone.")
	opts.RegisterDuration(&m.approvalGracePeriod, milestoneOptApprovalGracePeriod, 168*time.Hour, "The grace period to wait before removing a non-blocking issue/pr without sig approval from the active milestone.")
	opts.RegisterDuration(&m.slushUpdateInterval, milestoneOptSlushUpdateInterval, 72*time.Hour, "The expected interval, during code slush, between updates to a blocking issue/pr in the active milestone.")
	opts.RegisterDuration(&m.freezeUpdateInterval, milestoneOptFreezeUpdateInterval, 24*time.Hour, "The expected interval, during code freeze, between updates to a blocking issue/pr in the active milestone.")
	// Slush mode requires a freeze date to include in notifications
	// indicating the date by which non-critical issues must be closed
	// or upgraded in priority to avoid being moved out of the
	// milestone.  Only a single freeze date can be set under the
	// assumption that, where multiple milestones are targeted, only
	// one at a time will be in slush mode.
	opts.RegisterString(&m.freezeDate, milestoneOptFreezeDate, "", fmt.Sprintf("The date string indicating when code freeze will take effect."))

	opts.RegisterUpdateCallback(func(changed sets.String) error {
		for name, validator := range m.validators {
			if changed.Has(name) {
				if err := validator(name); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return nil
}

func (m *MilestoneMaintainer) updateInterval(mode string) time.Duration {
	if mode == milestoneModeSlush {
		return m.slushUpdateInterval
	}
	if mode == milestoneModeFreeze {
		return m.freezeUpdateInterval
	}
	return 0
}

// milestoneMode determines the release milestone and mode for the
// provided github object.  If a milestone is set and one of those
// targeted by the munger, the milestone and mode will be returned
// along with a boolean indication of success.  Otherwise, if the
// milestone is not set or not targeted, a boolean indication of
// failure will be returned.
func (m *MilestoneMaintainer) milestoneMode(obj *github.MungeObject) (milestone string, mode string, success bool) {
	// Ignore issues that lack an assigned milestone
	milestone, ok := obj.ReleaseMilestone()
	if !ok || len(milestone) == 0 {
		return "", "", false
	}

	// Ignore issues that aren't in a targeted milestone
	mode, exists := m.milestoneModeMap[milestone]
	if !exists {
		return "", "", false
	}
	return milestone, mode, true
}

// Munge is the workhorse the will actually make updates to the issue
func (m *MilestoneMaintainer) Munge(obj *github.MungeObject) {
	if ignoreObject(obj) {
		return
	}

	change := m.issueChange(obj)
	if change == nil {
		return
	}

	if !updateMilestoneStateLabel(obj, change.label) {
		return
	}

	comment, ok := latestNotificationComment(obj, m.botName)
	if !ok {
		return
	}
	if !notificationIsCurrent(change.notification, comment, change.commentInterval) {
		if comment != nil {
			if err := obj.DeleteComment(comment.Source.(*githubapi.IssueComment)); err != nil {
				return
			}
		}
		if err := change.notification.Post(obj); err != nil {
			return
		}
	}

	if change.removeFromMilestone {
		obj.ClearMilestone()
	}
}

// issueChange computes the changes required to modify the state of
// the issue to reflect the milestone process. If a nil return value
// is returned, no action should be taken.
func (m *MilestoneMaintainer) issueChange(obj *github.MungeObject) *issueChange {
	icc := m.issueChangeConfig(obj)
	if icc == nil {
		return nil
	}

	messageBody := icc.messageBody()
	if messageBody == nil {
		return nil
	}

	stateConfig := milestoneStateConfigs[icc.state]

	mentions := mungerutil.GetIssueUsers(obj.Issue).AllUsers().Mention().Join()
	if stateConfig.notifySIGs {
		sigMentions := icc.sigMentions()
		if len(sigMentions) > 0 {
			mentions = fmt.Sprintf("%s %s", mentions, sigMentions)
		}
	}

	message := fmt.Sprintf("%s\n\n%s\n%s", mentions, *messageBody, milestoneDetail)

	var commentInterval *time.Duration
	if stateConfig.warnOnInterval {
		commentInterval = &m.warningInterval
	}

	// Ensure the title refers to the correct type (issue or pr)
	title := fmt.Sprintf(stateConfig.title, strings.Title(objTypeString(obj)))

	return &issueChange{
		notification:        c.NewNotification(milestoneNotifierName, title, message),
		label:               stateConfig.label,
		removeFromMilestone: icc.state == milestoneNeedsRemoval,
		commentInterval:     commentInterval,
	}
}

// issueChangeConfig computes the configuration required to determine
// the changes to make to an issue so that it reflects the milestone
// process. If a nil return value is returned, no action should be
// taken.
func (m *MilestoneMaintainer) issueChangeConfig(obj *github.MungeObject) *issueChangeConfig {
	milestone, mode, ok := m.milestoneMode(obj)
	if !ok {
		return nil
	}

	updateInterval := m.updateInterval(mode)

	objType := objTypeString(obj)

	icc := &issueChangeConfig{
		enabledSections: sets.String{},
		templateArguments: map[string]interface{}{
			"approvalGracePeriod": durationToMaxDays(m.approvalGracePeriod),
			"approvedLabel":       quoteLabel(statusApprovedLabel),
			"blockerLabel":        quoteLabel(blockerLabel),
			"freezeDate":          m.freezeDate,
			"inProgressLabel":     quoteLabel(statusInProgressLabel),
			"labelGracePeriod":    durationToMaxDays(m.labelGracePeriod),
			"milestone":           fmt.Sprintf("%s milestone", milestone),
			"mode":                mode,
			"objType":             objType,
			"objTypePlural":       fmt.Sprintf("%ss", objType),
			"objTypeTitle":        strings.Title(objType),
			"updateInterval":      durationToMaxDays(updateInterval),
		},
		sigLabels: []string{},
	}

	isBlocker := obj.HasLabel(blockerLabel)

	if kind, priority, sigs, labelErrors := checkLabels(obj.Issue.Labels); len(labelErrors) == 0 {
		icc.summarizeLabels(objType, kind, priority, sigs)
		if !obj.HasLabel(statusApprovedLabel) {
			if isBlocker {
				icc.warnUnapproved(nil, objType, milestone)
			} else {
				removeAfter, ok := gracePeriodRemaining(obj, m.botName, milestoneNeedsApprovalLabel, m.approvalGracePeriod, time.Now(), false)
				if !ok {
					return nil
				}

				if removeAfter == nil || *removeAfter >= 0 {
					icc.warnUnapproved(removeAfter, objType, milestone)
				} else {
					icc.removeUnapproved()
				}
			}
			return icc
		}

		if mode == milestoneModeDev {
			// Status and updates are not required for dev mode
			return icc
		}

		if obj.IsPR() {
			// Status and updates are not required for PRs, and
			// non-blocking PRs should not be removed from the
			// milestone.
			return icc
		}

		if mode == milestoneModeFreeze && !isBlocker {
			icc.removeNonBlocker()
			return icc
		}

		if !obj.HasLabel(statusInProgressLabel) {
			icc.warnMissingInProgress()
		}

		if !isBlocker {
			icc.enableSection("warnNonBlockerRemoval")
		} else if updateInterval > 0 {
			lastUpdateTime, ok := findLastModificationTime(obj)
			if !ok {
				return nil
			}

			durationSinceUpdate := time.Since(*lastUpdateTime)
			if durationSinceUpdate > updateInterval {
				icc.warnUpdateRequired(*lastUpdateTime)
			}
			icc.enableSection("warnUpdateInterval")
		}
	} else {
		removeAfter, ok := gracePeriodRemaining(obj, m.botName, milestoneLabelsIncompleteLabel, m.labelGracePeriod, time.Now(), isBlocker)
		if !ok {
			return nil
		}

		if removeAfter == nil || *removeAfter >= 0 {
			icc.warnIncompleteLabels(removeAfter, labelErrors, objType, milestone)
		} else {
			icc.removeIncompleteLabels(labelErrors)
		}
	}
	return icc
}

func objTypeString(obj *github.MungeObject) string {
	if obj.IsPR() {
		return "pull request"
	}
	return "issue"
}

// issueChangeConfig is the config required to change an issue (via
// comments and labeling) to reflect the reuqirements of the milestone
// maintainer.
type issueChangeConfig struct {
	state             milestoneState
	enabledSections   sets.String
	sigLabels         []string
	templateArguments map[string]interface{}
}

func (icc *issueChangeConfig) messageBody() *string {
	for _, sectionName := range icc.enabledSections.List() {
		// If an issue will be removed from the milestone, suppress non-removal sections
		if icc.state != milestoneNeedsRemoval || strings.HasPrefix(sectionName, "remove") {
			icc.templateArguments[sectionName] = true
		}
	}

	icc.templateArguments["onlySummary"] = icc.state == milestoneCurrent

	return approvers.GenerateTemplateOrFail(milestoneMessageTemplate, "message", icc.templateArguments)
}

func (icc *issueChangeConfig) enableSection(sectionName string) {
	icc.enabledSections.Insert(sectionName)
}

func (icc *issueChangeConfig) summarizeLabels(objType, kindLabel, priorityLabel string, sigLabels []string) {
	icc.enableSection("summarizeLabels")
	icc.state = milestoneCurrent
	icc.sigLabels = sigLabels
	quotedSigLabels := []string{}
	for _, sigLabel := range sigLabels {
		quotedSigLabels = append(quotedSigLabels, quoteLabel(sigLabel))
	}
	arguments := map[string]interface{}{
		"kindLabel":           quoteLabel(kindLabel),
		"kindDescription":     kindMap[kindLabel],
		"priorityLabel":       quoteLabel(priorityLabel),
		"priorityDescription": fmt.Sprintf(priorityMap[priorityLabel], objType),
		"sigLabels":           quotedSigLabels,
	}
	for k, v := range arguments {
		icc.templateArguments[k] = v
	}
}

func (icc *issueChangeConfig) warnUnapproved(removeAfter *time.Duration, objType, milestone string) {
	icc.enableSection("warnUnapproved")
	icc.state = milestoneNeedsApproval
	var warning string
	if removeAfter != nil {
		warning = fmt.Sprintf(" If the label is not applied within %s, the %s will be moved out of the %s milestone.",
			durationToMaxDays(*removeAfter), objType, milestone)
	}
	icc.templateArguments["unapprovedRemovalWarning"] = warning

}

func (icc *issueChangeConfig) removeUnapproved() {
	icc.enableSection("removeUnapproved")
	icc.state = milestoneNeedsRemoval
}

func (icc *issueChangeConfig) removeNonBlocker() {
	icc.enableSection("removeNonBlocker")
	icc.state = milestoneNeedsRemoval
}

func (icc *issueChangeConfig) warnMissingInProgress() {
	icc.enableSection("warnMissingInProgress")
	icc.state = milestoneNeedsAttention
}

func (icc *issueChangeConfig) warnUpdateRequired(lastUpdated time.Time) {
	icc.enableSection("warnUpdateRequired")
	icc.state = milestoneNeedsAttention
	icc.templateArguments["lastUpdated"] = lastUpdated.Format("Jan 2")
}

func (icc *issueChangeConfig) warnIncompleteLabels(removeAfter *time.Duration, labelErrors []string, objType, milestone string) {
	icc.enableSection("warnIncompleteLabels")
	icc.state = milestoneNeedsLabeling
	var warning string
	if removeAfter != nil {
		warning = fmt.Sprintf(" If the required changes are not made within %s, the %s will be moved out of the %s milestone.",
			durationToMaxDays(*removeAfter), objType, milestone)
	}
	icc.templateArguments["incompleteLabelsRemovalWarning"] = warning
	icc.templateArguments["labelErrors"] = labelErrors
}

func (icc *issueChangeConfig) removeIncompleteLabels(labelErrors []string) {
	icc.enableSection("removeIncompleteLabels")
	icc.state = milestoneNeedsRemoval
	icc.templateArguments["labelErrors"] = labelErrors
}

func (icc *issueChangeConfig) sigMentions() string {
	mentions := []string{}
	for _, label := range icc.sigLabels {
		sig := strings.TrimPrefix(label, sigLabelPrefix)
		target := fmt.Sprintf(sigMentionTemplate, sig)
		mentions = append(mentions, target)
	}
	return strings.Join(mentions, " ")
}

// ignoreObject indicates whether the munger should ignore the given
// object.
func ignoreObject(obj *github.MungeObject) bool {
	// Ignore closed
	if obj.Issue.State != nil && *obj.Issue.State == "closed" {
		return true
	}

	return false
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

// notificationIsCurrent indicates whether the given notification
// matches the most recent notification comment and the comment
// interval - if provided - has not been exceeded.
func notificationIsCurrent(notification *c.Notification, comment *c.Comment, commentInterval *time.Duration) bool {
	oldNotification := c.ParseNotification(comment)
	notificationsEqual := oldNotification != nil && oldNotification.Equal(notification)
	return notificationsEqual && (commentInterval == nil || comment != nil && comment.CreatedAt != nil && time.Since(*comment.CreatedAt) < *commentInterval)
}

// gracePeriodRemaining returns the difference between the start of
// the grace period and the grace period interval. Returns nil the
// grace period start cannot be determined.
func gracePeriodRemaining(obj *github.MungeObject, botName, labelName string, gracePeriod time.Duration, defaultStart time.Time, isBlocker bool) (*time.Duration, bool) {
	if isBlocker {
		return nil, true
	}
	tempStart := gracePeriodStart(obj, botName, labelName, defaultStart)
	if tempStart == nil {
		return nil, false
	}
	start := *tempStart

	remaining := -time.Since(start.Add(gracePeriod))
	return &remaining, true
}

// gracePeriodStart determines when the grace period for the given
// object should start as is indicated by when the
// milestone-labels-incomplete label was last applied. If the label
// is not set, the default will be returned. nil will be returned if
// an error occurs while accessing the object's label events.
func gracePeriodStart(obj *github.MungeObject, botName, labelName string, defaultStart time.Time) *time.Time {
	if !obj.HasLabel(labelName) {
		return &defaultStart
	}

	return labelLastCreatedAt(obj, botName, labelName)
}

// labelLastCreatedAt returns the time at which the given label was
// last applied to the given github object. Returns nil if an error
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
		kindLabels := formatLabelString(kindMap)
		labelErrors = append(labelErrors, fmt.Sprintf("_**kind**_: Must specify exactly one of %s.", kindLabels))
	}

	priorityLabel, err = uniqueLabelName(labels, priorityMap)
	if err != nil || len(priorityLabel) == 0 {
		priorityLabels := formatLabelString(priorityMap)
		labelErrors = append(labelErrors, fmt.Sprintf("_**priority**_: Must specify exactly one of %s.", priorityLabels))
	}

	sigLabels = sigLabelNames(labels)
	if len(sigLabels) == 0 {
		labelErrors = append(labelErrors, fmt.Sprintf("_**sig owner**_: Must specify at least one label prefixed with `%s`.", sigLabelPrefix))
	}

	return
}

// uniqueLabelName determines which label of a set indicated by a map
// - if any - is present in the given slice of labels. Returns an
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
		if strings.HasPrefix(*label.Name, sigLabelPrefix) {
			labelNames = append(labelNames, *label.Name)
		}
	}
	return labelNames
}

// formatLabelString converts a map to a string in the format "`key-foo`, `key-bar`".
func formatLabelString(labelMap map[string]string) string {
	labelList := []string{}
	for k := range labelMap {
		labelList = append(labelList, quoteLabel(k))
	}
	sort.Strings(labelList)

	maxIndex := len(labelList) - 1
	if maxIndex == 0 {
		return labelList[0]
	}
	return strings.Join(labelList[0:maxIndex], ", ") + " or " + labelList[maxIndex]
}

// quoteLabel formats a label name as inline code in markdown (e.g. `labelName`)
func quoteLabel(label string) string {
	if len(label) > 0 {
		return fmt.Sprintf("`%s`", label)
	}
	return label
}

// updateMilestoneStateLabel ensures that the given milestone state
// label is the only state label set on the given issue.
func updateMilestoneStateLabel(obj *github.MungeObject, labelName string) bool {
	if len(labelName) > 0 && !obj.HasLabel(labelName) {
		if err := obj.AddLabel(labelName); err != nil {
			return false
		}
	}
	for _, stateLabel := range milestoneStateLabels {
		if stateLabel != labelName && obj.HasLabel(stateLabel) {
			if err := obj.RemoveLabel(stateLabel); err != nil {
				return false
			}
		}
	}
	return true
}
