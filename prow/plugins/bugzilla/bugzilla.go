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

// Package bugzilla ensures that pull requests reference a Bugzilla bug in their title
package bugzilla

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/bugzilla"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

var (
	titleMatch   = regexp.MustCompile(`(?i)^.*?Bug ([0-9]+):`)
	commandMatch = regexp.MustCompile(`(?mi)^/bugzilla refresh\s*$`)
)

const (
	PluginName = "bugzilla"
	bugLink    = `[Bugzilla bug](%s/show_bug.cgi?id=%d)`
)

func init() {
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericComment, helpProvider)
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	configInfo := make(map[string]string)
	for _, orgRepo := range enabledRepos {
		parts := strings.Split(orgRepo, "/")
		var opts map[string]plugins.BugzillaBranchOptions
		switch len(parts) {
		case 2:
			opts = config.Bugzilla.OptionsForRepo(parts[0], parts[1])
		default:
			return nil, fmt.Errorf("invalid repo in enabledRepos: %q", orgRepo)
		}
		if len(opts) == 0 {
			continue
		}
		// we need to make sure the order of this help is consistent for page reloads and testing
		var branches []string
		for branch := range opts {
			branches = append(branches, branch)
		}
		sort.Strings(branches)
		var configInfoStrings []string
		configInfoStrings = append(configInfoStrings, "The plugin has the following configuration:<ul>")
		for _, branch := range branches {
			var message string
			if branch == plugins.BugzillaOptionsWildcard {
				message = "by default, "
			} else {
				message = fmt.Sprintf("on the %q branch, ", branch)
			}
			message += "valid bugs must "
			var conditions []string
			if opts[branch].IsOpen != nil {
				if *opts[branch].IsOpen {
					conditions = append(conditions, "be open")
				} else {
					conditions = append(conditions, "be closed")
				}
			}
			if opts[branch].TargetRelease != nil {
				conditions = append(conditions, fmt.Sprintf("target the %q release", *opts[branch].TargetRelease))
			}
			if opts[branch].Statuses != nil {
				conditions = append(conditions, fmt.Sprintf("be in one of the following states: %s", strings.Join(*opts[branch].Statuses, ", ")))
			}
			switch len(conditions) {
			case 0:
				message += "exist"
			case 1:
				message += conditions[0]
			case 2:
				message += fmt.Sprintf("%s and %s", conditions[0], conditions[1])
			default:
				conditions[len(conditions)-1] = fmt.Sprintf("and %s", conditions[len(conditions)-1])
				message += strings.Join(conditions, ", ")
			}
			var updates []string
			if opts[branch].StatusAfterValidation != nil {
				updates = append(updates, fmt.Sprintf("moved to the %q state", *opts[branch].StatusAfterValidation))
			}
			if opts[branch].AddExternalLink != nil && *opts[branch].AddExternalLink {
				updates = append(updates, "updated to refer to the pull request using the external bug tracker")
			}
			if opts[branch].StatusAfterMerge != nil {
				updates = append(updates, fmt.Sprintf("moved to the %q state when all linked pull requests are merged", *opts[branch].StatusAfterMerge))
			}

			if len(updates) > 0 {
				message += ". After being linked to a pull request, bugs will be "
			}
			switch len(updates) {
			case 0:
			case 1:
				message += updates[0]
			case 2:
				message += fmt.Sprintf("%s and %s", updates[0], updates[1])
			default:
				updates[len(updates)-1] = fmt.Sprintf("and %s", updates[len(updates)-1])
				message += strings.Join(updates, ", ")
			}
			configInfoStrings = append(configInfoStrings, "<li>"+message+".</li>")
		}
		configInfoStrings = append(configInfoStrings, "</ul>")

		configInfo[orgRepo] = strings.Join(configInfoStrings, "\n")
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The bugzilla plugin ensures that pull requests reference a valid Bugzilla bug in their title.",
		Config:      configInfo,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/bugzilla refresh",
		Description: "Check Bugzilla for a valid bug referenced in the PR title",
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/bugzilla refresh"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	CreateComment(owner, repo string, number int, comment string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	event, err := digestComment(pc.GitHubClient, pc.Logger, e)
	if err != nil {
		return err
	}
	if event != nil {
		options := pc.PluginConfig.Bugzilla.OptionsForBranch(event.org, event.repo, event.baseRef)
		return handle(*event, pc.GitHubClient, pc.BugzillaClient, options, pc.Logger)
	}
	return nil
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	options := pc.PluginConfig.Bugzilla.OptionsForBranch(pre.PullRequest.Base.Repo.Owner.Login, pre.PullRequest.Base.Repo.Name, pre.PullRequest.Base.Ref)
	event, err := digestPR(pc.Logger, pre, options.ValidateByDefault)
	if err != nil {
		return err
	}
	if event != nil {
		return handle(*event, pc.GitHubClient, pc.BugzillaClient, options, pc.Logger)
	}
	return nil
}

// digestPR determines if any action is necessary and creates the objects for handle() if it is
func digestPR(log *logrus.Entry, pre github.PullRequestEvent, validateByDefault *bool) (*event, error) {
	// These are the only actions indicating the PR title may have changed or that the PR merged
	if pre.Action != github.PullRequestActionOpened &&
		pre.Action != github.PullRequestActionReopened &&
		pre.Action != github.PullRequestActionEdited &&
		!(pre.Action == github.PullRequestActionClosed && pre.PullRequest.Merged) {
		return nil, nil
	}

	var (
		org     = pre.PullRequest.Base.Repo.Owner.Login
		repo    = pre.PullRequest.Base.Repo.Name
		baseRef = pre.PullRequest.Base.Ref
		number  = pre.PullRequest.Number
		title   = pre.PullRequest.Title
	)

	// Make sure the PR title is referencing a bug
	e := &event{org: org, repo: repo, baseRef: baseRef, number: number, merged: pre.PullRequest.Merged, body: title, htmlUrl: pre.PullRequest.HTMLURL, login: pre.PullRequest.User.Login}
	mat := titleMatch.FindStringSubmatch(title)
	if mat == nil {
		// in the case that the title used to reference a bug and no longer does we
		// want to handle this to remove labels
		e.missing = true
	} else {
		id, err := strconv.Atoi(mat[1])
		if err != nil {
			// should be impossible based on the regex
			log.WithError(err).Debug("Failed to parse bug ID as int - is the regex correct?")
			return nil, err
		}
		e.bugId = id
	}

	// when exiting early from errors trying to find out if the PR previously referenced a bug,
	// we want to handle the event only if a bug is currently referenced or we are validating by
	// default
	var intermediate *event
	if !e.missing || (validateByDefault != nil && *validateByDefault) {
		intermediate = e
	}

	// Check if the previous version of the title referenced a bug.
	var changes struct {
		Title struct {
			From string `json:"from"`
		} `json:"title"`
	}
	if err := json.Unmarshal(pre.Changes, &changes); err != nil {
		// we're detecting this best-effort so we can handle it anyway
		return intermediate, nil
	}
	prevMat := titleMatch.FindStringSubmatch(changes.Title.From)
	if prevMat == nil {
		// title did not previously reference a bug
		return intermediate, nil
	}
	prevId, err := strconv.Atoi(prevMat[1])
	if err != nil {
		// should be impossible based on the regex, ignore err as this is best-effort
		log.WithError(err).Debug("Failed to parse bug ID as int - is the regex correct?")
		return intermediate, nil
	}

	// if the referenced bug has not changed in the update, ignore it
	if prevId == e.bugId {
		logrus.Debugf("Referenced Bugzilla ID (%d) has not changed, not handling event.", e.bugId)
		return nil, nil
	}

	// we know the PR previously referenced a bug, so whether
	// it currently does or does not reference a bug, we should
	// handle the event
	return e, nil
}

// digestComment determines if any action is necessary and creates the objects for handle() if it is
func digestComment(gc githubClient, log *logrus.Entry, gce github.GenericCommentEvent) (*event, error) {
	// Only consider new comments.
	if gce.Action != github.GenericCommentActionCreated {
		return nil, nil
	}
	// Make sure they are requesting a bug refresh
	if !commandMatch.MatchString(gce.Body) {
		return nil, nil
	}
	var (
		org    = gce.Repo.Owner.Login
		repo   = gce.Repo.Name
		number = gce.Number
	)

	// We don't support linking issues to Bugs
	if !gce.IsPR {
		log.Debug("Bug refresh requested on an issue, ignoring")
		return nil, gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(gce.Body, gce.HTMLURL, gce.User.Login, `Bugzilla bug referencing is only supported for Pull Requests, not issues.`))
	}

	// Make sure the PR title is referencing a bug
	pr, err := gc.GetPullRequest(org, repo, number)
	if err != nil {
		return nil, err
	}

	e := &event{org: org, repo: repo, baseRef: pr.Base.Ref, number: number, merged: pr.Merged, body: gce.Body, htmlUrl: gce.HTMLURL, login: gce.User.Login}
	mat := titleMatch.FindStringSubmatch(pr.Title)
	if mat == nil {
		e.missing = true
		return e, nil
	}
	id, err := strconv.Atoi(mat[1])
	if err != nil {
		// should be impossible based on the regex
		log.WithError(err).Debug("Failed to parse bug ID as int - is the regex correct?")
		return nil, err
	}
	e.bugId = id

	return e, nil
}

type event struct {
	org, repo, baseRef   string
	number, bugId        int
	missing, merged      bool
	body, htmlUrl, login string
}

func (e *event) comment(gc githubClient) func(body string) error {
	return func(body string) error {
		return gc.CreateComment(e.org, e.repo, e.number, plugins.FormatResponseRaw(e.body, e.htmlUrl, e.login, body))
	}
}

func handle(e event, gc githubClient, bc bugzilla.Client, options plugins.BugzillaBranchOptions, log *logrus.Entry) error {
	comment := e.comment(gc)
	// merges follow a different pattern from the normal validation
	if e.merged {
		return handleMerge(e, gc, bc, options, log)
	}

	var needsValidLabel, needsInvalidLabel bool
	var response string
	if e.missing {
		log.WithField("bugMissing", true)
		log.Debug("No bug referenced.")
		needsValidLabel, needsInvalidLabel = false, false
		response = `No Bugzilla bug is referenced in the title of this pull request.
To reference a bug, add 'Bug XXX:' to the title of this pull request and request another bug refresh with <code>/bugzilla refresh</code>.`
	} else {
		log = log.WithField("bugId", e.bugId)

		bug, err := getBug(bc, e.bugId, log, comment)
		if err != nil || bug == nil {
			return err
		}

		var dependents []bugzilla.Bug
		if options.DependentBugStatuses != nil {
			for _, id := range bug.DependsOn {
				dependent, err := bc.GetBug(id)
				if err != nil {
					return comment(fmt.Sprintf(`An error was encountered searching the Bugzilla server at %s for dependent bug %d:
> %v
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.`,
						bc.Endpoint(), id, err))
				}
				dependents = append(dependents, *dependent)
			}
		}

		valid, why := validateBug(*bug, dependents, options, bc.Endpoint())
		needsValidLabel, needsInvalidLabel = valid, !valid
		if valid {
			log.Debug("Valid bug found.")
			response = fmt.Sprintf(`This pull request references a valid `+bugLink+`.`, bc.Endpoint(), e.bugId)
			// if configured, move the bug to the new state
			if options.StatusAfterValidation != nil && bug.Status != *options.StatusAfterValidation {
				if err := bc.UpdateBug(e.bugId, bugzilla.BugUpdate{Status: *options.StatusAfterValidation}); err != nil {
					log.WithError(err).Warn("Unexpected error updating Bugzilla bug.")
					return comment(fmt.Sprintf(`An error was encountered updating the bug to the %s state on the Bugzilla server at %s for bug %d:
> %v
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.`,
						*options.StatusAfterValidation, bc.Endpoint(), e.bugId, err))
				}
				response += fmt.Sprintf(" The bug has been moved to the %s state.", *options.StatusAfterValidation)
			}
			if options.AddExternalLink != nil && *options.AddExternalLink {
				changed, err := bc.AddPullRequestAsExternalBug(e.bugId, e.org, e.repo, e.number)
				if err != nil {
					log.WithError(err).Warn("Unexpected error adding external tracker bug to Bugzilla bug.")
					return comment(fmt.Sprintf(`An error was encountered adding this pull request to the external tracker bugs on the Bugzilla server at %s for bug %d:
> %v
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.`,
						bc.Endpoint(), e.bugId, err))
				}
				if changed {
					response += " The bug has been updated to refer to the pull request using the external bug tracker."
				}
			}
		} else {
			log.Debug("Invalid bug found.")
			var formattedReasons string
			for _, reason := range why {
				formattedReasons += fmt.Sprintf(" - %s\n", reason)
			}
			response = fmt.Sprintf(`This pull request references an invalid `+bugLink+`:
%s
Comment <code>/bugzilla refresh</code> to re-evaluate validity if changes to the Bugzilla bug are made, or edit the title of this pull request to link to a different bug.`, bc.Endpoint(), e.bugId, formattedReasons)
		}
	}

	// ensure label state is correct. Do not propagate errors
	// as it is more important to report to the user than to
	// fail early on a label check.
	currentLabels, err := gc.GetIssueLabels(e.org, e.repo, e.number)
	if err != nil {
		log.WithError(err).Warn("Could not list labels on PR")
	}
	var hasValidLabel, hasInvalidLabel bool
	for _, l := range currentLabels {
		if l.Name == labels.ValidBug {
			hasValidLabel = true
		}
		if l.Name == labels.InvalidBug {
			hasInvalidLabel = true
		}
	}

	if needsValidLabel && !hasValidLabel {
		if err := gc.AddLabel(e.org, e.repo, e.number, labels.ValidBug); err != nil {
			log.WithError(err).Error("Failed to add valid bug label.")
		}
	} else if !needsValidLabel && hasValidLabel {
		if err := gc.RemoveLabel(e.org, e.repo, e.number, labels.ValidBug); err != nil {
			log.WithError(err).Error("Failed to remove valid bug label.")
		}
	}

	if needsInvalidLabel && !hasInvalidLabel {
		if err := gc.AddLabel(e.org, e.repo, e.number, labels.InvalidBug); err != nil {
			log.WithError(err).Error("Failed to add invalid bug label.")
		}
	} else if !needsInvalidLabel && hasInvalidLabel {
		if err := gc.RemoveLabel(e.org, e.repo, e.number, labels.InvalidBug); err != nil {
			log.WithError(err).Error("Failed to remove invalid bug label.")
		}
	}

	return comment(response)
}

// validateBug determines if the bug matches the options and returns a description of why not
func validateBug(bug bugzilla.Bug, dependents []bugzilla.Bug, options plugins.BugzillaBranchOptions, endpoint string) (bool, []string) {
	valid := true
	var errors []string
	if options.IsOpen != nil && *options.IsOpen != bug.IsOpen {
		valid = false
		not := ""
		was := "isn't"
		if !*options.IsOpen {
			not = "not "
			was = "is"
		}
		errors = append(errors, fmt.Sprintf("expected the bug to %sbe open, but it %s", not, was))
	}

	if options.TargetRelease != nil {
		if len(bug.TargetRelease) == 0 {
			valid = false
			errors = append(errors, fmt.Sprintf("expected the bug to target the %q release, but no target release was set", *options.TargetRelease))
		} else if *options.TargetRelease != bug.TargetRelease[0] {
			// the BugZilla web UI shows one option for target release, but returns the
			// field as a list in the REST API. We only care for the first item and it's
			// not even clear if the list can have more than one item in the response
			valid = false
			errors = append(errors, fmt.Sprintf("expected the bug to target the %q release, but it targets %q instead", *options.TargetRelease, bug.TargetRelease[0]))
		}
	}

	if options.Statuses != nil {
		validStatuses := sets.NewString(*options.Statuses...)
		if options.StatusAfterValidation != nil {
			validStatuses.Insert(*options.StatusAfterValidation)
		}
		if !validStatuses.Has(bug.Status) {
			valid = false
			errors = append(errors, fmt.Sprintf("expected the bug to be in one of the following states: %s, but it is %s instead", strings.Join(*options.Statuses, ", "), bug.Status))
		}
	}

	if options.DependentBugStatuses != nil {
		validStatuses := sets.NewString(*options.DependentBugStatuses...)
		for _, bug := range dependents {
			if !validStatuses.Has(bug.Status) {
				valid = false
				errors = append(errors, fmt.Sprintf("expected dependent "+bugLink+" to be in one of the following states: %s, but it is %s instead", endpoint, bug.ID, strings.Join(*options.DependentBugStatuses, ", "), bug.Status))
			}
		}
	}

	return valid, errors
}

func handleMerge(e event, gc githubClient, bc bugzilla.Client, options plugins.BugzillaBranchOptions, log *logrus.Entry) error {
	comment := e.comment(gc)

	if options.StatusAfterMerge == nil {
		return nil
	}
	if e.missing {
		return nil
	}
	if options.Statuses != nil || options.StatusAfterValidation != nil {
		// we should only migrate if we can be fairly certain that the bug
		// is not in a state that required human intervention to get to.
		// For instance, if a bug is closed after a PR merges it should not
		// be possible for /bugzilla refresh to move it back to the post-merge
		// state.
		bug, err := getBug(bc, e.bugId, log, comment)
		if err != nil || bug == nil {
			return err
		}
		validStatuses := sets.NewString()
		if options.Statuses != nil {
			validStatuses.Insert(*options.Statuses...)
		}
		if options.StatusAfterValidation != nil {
			validStatuses.Insert(*options.StatusAfterValidation)
		}
		if !validStatuses.Has(bug.Status) {
			return comment(fmt.Sprintf("The "+bugLink+" is in an unrecognized state (%s) and will not be moved to the %s state.", bc.Endpoint(), e.bugId, bug.Status, *options.StatusAfterMerge))
		}
	}

	prs, err := bc.GetExternalBugPRsOnBug(e.bugId)
	if err != nil {
		log.WithError(err).Warn("Unexpected error listing external tracker bugs for Bugzilla bug.")
		return comment(fmt.Sprintf(`An error was encountered searching the Bugzilla server at %s for external trackers on bug %d:
> %v
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.`,
			bc.Endpoint(), e.bugId, err))
	}
	shouldMigrate := true
	for _, item := range prs {
		var merged bool
		if e.org == item.Org && e.repo == item.Repo && e.number == item.Num {
			merged = e.merged
		} else {
			pr, err := gc.GetPullRequest(item.Org, item.Repo, item.Num)
			if err != nil {
				log.WithError(err).Warn("Unexpected error checking merge state of related pull request.")
				return comment(fmt.Sprintf(`An error was encountered checking the state of a related pull request at https://github.com/%s/%s/pull/%d for bug %d:
> %v
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.`,
					item.Org, item.Repo, item.Num, e.bugId, err))
			}
			merged = pr.Merged
		}
		// only update Bugzilla bug status if all PRs have merged
		shouldMigrate = shouldMigrate && merged
		if !shouldMigrate {
			break
		}
	}

	if shouldMigrate {
		if err := bc.UpdateBug(e.bugId, bugzilla.BugUpdate{Status: *options.StatusAfterMerge}); err != nil {
			log.WithError(err).Warn("Unexpected error updating Bugzilla bug.")
			return comment(fmt.Sprintf(`An error was encountered updating the bug to the %s state on the Bugzilla server at %s for bug %d:
> %v
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.`,
				*options.StatusAfterMerge, bc.Endpoint(), e.bugId, err))
		}
		return comment(fmt.Sprintf("All pull requests linked via external trackers have merged. The "+bugLink+" has been moved to the %s state.", bc.Endpoint(), e.bugId, *options.StatusAfterMerge))
	}
	return nil
}

func getBug(bc bugzilla.Client, bugId int, log *logrus.Entry, comment func(string) error) (*bugzilla.Bug, error) {
	bug, err := bc.GetBug(bugId)
	if err != nil && !bugzilla.IsNotFound(err) {
		log.WithError(err).Warn("Unexpected error searching for Bugzilla bug.")
		return nil, comment(fmt.Sprintf(`An error was encountered searching the Bugzilla server at %s for bug %d:
> %v
Please contact an administrator to resolve this issue, then request a bug refresh with <code>/bugzilla refresh</code>.`,
			bc.Endpoint(), bugId, err))
	}
	if bugzilla.IsNotFound(err) || bug == nil {
		log.Debug("No bug found.")
		return nil, comment(fmt.Sprintf(`No Bugzilla bug with ID %d exists in the tracker at %s.
Once a valid bug is referenced in the title of this pull request, request a bug refresh with <code>/bugzilla refresh</code>.`,
			bugId, bc.Endpoint()))
	}
	return bug, nil
}
