/*
Copyright 2020 The Kubernetes Authors.

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

package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/andygrunwald/go-jira"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"github.com/trivago/tgo/tcontainer"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	jiraclient "k8s.io/test-infra/prow/jira"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	PluginName            = "jira"
	bugLink               = `[Jira Issue %s](%s/browse/%s)`
	criticalSeverity      = "Critical"
	importantSeverity     = "Important"
	moderateSeverity      = "Moderate"
	lowSeverity           = "Low"
	informationalSeverity = "Informational"
)

var (
	titleMatch           = regexp.MustCompile(`(?i)OCPBUGS-([0-9]+):`)
	refreshCommandMatch  = regexp.MustCompile(`(?mi)^/jira refresh\s*$`)
	qaReviewCommandMatch = regexp.MustCompile(`(?mi)^/jira cc-qa\s*$`)
	cherrypickPRMatch    = regexp.MustCompile(`This is an automated cherry-pick of #([0-9]+)`)
)

var (
	issueNameRegex = regexp.MustCompile(`\b([a-zA-Z]+-[0-9]+)(\s|:|$)`)
	projectCache   = &threadsafeSet{data: sets.String{}}
)

func extractCandidatesFromText(t string) []string {
	matches := issueNameRegex.FindAllStringSubmatch(t, -1)
	if matches == nil {
		return nil
	}
	var result []string
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		result = append(result, match[1])
	}
	return result
}

func init() {
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericComment, helpProvider)
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	configInfo := make(map[string]string)
	for _, repo := range enabledRepos {
		opts := config.Jira.OptionsForRepo(repo.Org, repo.Repo)
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
			if branch == plugins.JiraOptionsWildcard {
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
			if opts[branch].TargetVersion != nil {
				conditions = append(conditions, fmt.Sprintf("target the %q version", *opts[branch].TargetVersion))
			}
			if opts[branch].ValidStates != nil && len(*opts[branch].ValidStates) > 0 {
				pretty := strings.Join(prettyStates(*opts[branch].ValidStates), ", ")
				conditions = append(conditions, fmt.Sprintf("be in one of the following states: %s", pretty))
			}
			if opts[branch].DependentBugStates != nil || opts[branch].DependentBugTargetVersions != nil {
				conditions = append(conditions, "depend on at least one other bug")
			}
			if opts[branch].DependentBugStates != nil {
				pretty := strings.Join(prettyStates(*opts[branch].DependentBugStates), ", ")
				conditions = append(conditions, fmt.Sprintf("have all dependent bugs in one of the following states: %s", pretty))
			}
			if opts[branch].DependentBugTargetVersions != nil {
				conditions = append(conditions, fmt.Sprintf("have all dependent bugs in one of the following target versions: %s", strings.Join(*opts[branch].DependentBugTargetVersions, ", ")))
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
			if opts[branch].StateAfterValidation != nil {
				updates = append(updates, fmt.Sprintf("moved to the %s state", opts[branch].StateAfterValidation))
			}
			if opts[branch].AddExternalLink != nil && *opts[branch].AddExternalLink {
				updates = append(updates, "updated to refer to the pull request using the external bug tracker")
			}
			if opts[branch].StateAfterMerge != nil {
				updates = append(updates, fmt.Sprintf("moved to the %s state when all linked pull requests are merged", opts[branch].StateAfterMerge))
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

		configInfo[repo.String()] = strings.Join(configInfoStrings, "\n")
	}
	str := func(s string) *string { return &s }
	yes := true
	no := false
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Jira: plugins.Jira{
			Default: map[string]plugins.JiraBranchOptions{
				"*": {
					ValidateByDefault: &yes,
					IsOpen:            &yes,
					TargetVersion:     str("version1"),
					ValidStates: &[]plugins.JiraBugState{
						{
							Status: "MODIFIED",
						},
						{
							Status:     "CLOSED",
							Resolution: "ERRATA",
						},
					},
					DependentBugStates: &[]plugins.JiraBugState{
						{
							Status: "MODIFIED",
						},
					},
					DependentBugTargetVersions: &[]string{"version1", "version2"},
					StateAfterValidation: &plugins.JiraBugState{
						Status: "VERIFIED",
					},
					AddExternalLink: &no,
					StateAfterMerge: &plugins.JiraBugState{
						Status:     "RELEASE_PENDING",
						Resolution: "RESOLVED",
					},
					StateAfterClose: &plugins.JiraBugState{
						Status:     "RESET",
						Resolution: "FIXED",
					},
					AllowedSecurityLevels: []string{"group1", "groups2"},
				},
			},
			Orgs: map[string]plugins.JiraOrgOptions{
				"org": {
					Default: map[string]plugins.JiraBranchOptions{
						"*": {
							ExcludeDefaults:   &yes,
							ValidateByDefault: &yes,
							IsOpen:            &yes,
							TargetVersion:     str("version1"),
							ValidStates: &[]plugins.JiraBugState{
								{
									Status: "MODIFIED",
								},
								{
									Status:     "CLOSED",
									Resolution: "ERRATA",
								},
							},
							DependentBugStates: &[]plugins.JiraBugState{
								{
									Status: "MODIFIED",
								},
							},
							DependentBugTargetVersions: &[]string{"version1", "version2"},
							StateAfterValidation: &plugins.JiraBugState{
								Status: "VERIFIED",
							},
							AddExternalLink: &no,
							StateAfterMerge: &plugins.JiraBugState{
								Status:     "RELEASE_PENDING",
								Resolution: "RESOLVED",
							},
							StateAfterClose: &plugins.JiraBugState{
								Status:     "RESET",
								Resolution: "FIXED",
							},
							AllowedSecurityLevels: []string{"group1", "groups2"},
						},
					},
					Repos: map[string]plugins.JiraRepoOptions{
						"repo": {
							Branches: map[string]plugins.JiraBranchOptions{
								"branch": {
									ExcludeDefaults:   &no,
									ValidateByDefault: &yes,
									IsOpen:            &yes,
									TargetVersion:     str("version1"),
									ValidStates: &[]plugins.JiraBugState{
										{
											Status: "MODIFIED",
										},
										{
											Status:     "CLOSED",
											Resolution: "ERRATA",
										},
									},
									DependentBugStates: &[]plugins.JiraBugState{
										{
											Status: "MODIFIED",
										},
									},
									DependentBugTargetVersions: &[]string{"version1", "version2"},
									StateAfterValidation: &plugins.JiraBugState{
										Status: "VERIFIED",
									},
									AddExternalLink: &no,
									StateAfterMerge: &plugins.JiraBugState{
										Status:     "RELEASE_PENDING",
										Resolution: "RESOLVED",
									},
									StateAfterClose: &plugins.JiraBugState{
										Status:     "RESET",
										Resolution: "FIXED",
									},
									AllowedSecurityLevels: []string{"group1", "groups2"},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", PluginName)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The jira plugin ensures that pull requests reference a valid Jira bug in their title.",
		Config:      configInfo,
		Snippet:     yamlSnippet,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/jira refresh",
		Description: "Check Jira for a valid bug referenced in the PR title",
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/jira refresh"},
	})
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/jira cc-qa",
		Description: "Request PR review from QA contact specified in Jira",
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/jira cc-qa"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	EditComment(org, repo string, id int, comment string) error
	GetIssue(org, repo string, number int) (*github.Issue, error)
	EditIssue(org, repo string, number int, issue *github.Issue) (*github.Issue, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	CreateComment(owner, repo string, number int, comment string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	WasLabelAddedByHuman(org, repo string, num int, label string) (bool, error)
	Query(ctx context.Context, q interface{}, vars map[string]interface{}) error
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered panic in jira plugin: %v", r)
		}
	}()
	event, err := digestComment(pc.GitHubClient, pc.Logger, e)
	if err != nil {
		return err
	}
	if event != nil {
		options := pc.PluginConfig.Jira.OptionsForBranch(event.org, event.repo, event.baseRef)
		return handle(pc.JiraClient, pc.GitHubClient, pc.PluginConfig.Jira.DisabledJiraProjects, options, pc.Logger, *event, pc.Config.AllRepos)
	}
	return nil
}

func handle(jc jiraclient.Client, ghc githubClient, disabledJiraProjects []string, options plugins.JiraBranchOptions, log *logrus.Entry, e event, allRepos sets.String) error {
	if projectCache.entryCount() == 0 {
		projects, err := jc.ListProjects()
		if err != nil {
			return fmt.Errorf("failed to list jira projects: %w", err)
		}
		var projectNames []string
		for _, project := range *projects {
			projectNames = append(projectNames, strings.ToLower(project.Key))
		}
		projectCache.insert(projectNames...)
	}

	return handleWithProjectCache(jc, ghc, disabledJiraProjects, options, log, e, allRepos, projectCache)
}

func handleWithProjectCache(jc jiraclient.Client, gc githubClient, disabledJiraProjects []string, options plugins.JiraBranchOptions, log *logrus.Entry, e event, allRepos sets.String, projectCache *threadsafeSet) error {
	jc = &projectCachingJiraClient{jc, projectCache}

	// if this is a comment, linkify the comment and add remote link for non-bug type issues
	if e.isComment {
		issueCandidateNames := extractCandidatesFromText(e.body)
		issueCandidateNames = append(issueCandidateNames, extractCandidatesFromText(e.title)...)
		issueCandidateNames = filterOutDisabledJiraProjects(issueCandidateNames, disabledJiraProjects)
		if len(issueCandidateNames) == 0 {
			return nil
		}

		var errs []error
		referencedIssues := sets.String{}
		for _, match := range issueCandidateNames {
			if referencedIssues.Has(match) {
				continue
			}
			_, err := jc.GetIssue(match)
			if err != nil {
				if !jiraclient.IsNotFound(err) {
					errs = append(errs, fmt.Errorf("failed to get issue %s: %w", match, err))
				}
				continue
			}
			referencedIssues.Insert(match)
		}

		wg := &sync.WaitGroup{}
		for _, issue := range referencedIssues.List() {
			// don't handle bug (i.e. OCPBUGS issues); those are handled separately
			if strings.HasPrefix(issue, "OCPBUGS-") {
				continue
			}
			wg.Add(1)
			go func(issue string) {
				defer wg.Done()
				if _, err := upsertGitHubLinkToIssue(log, issue, jc, e); err != nil {
					log.WithField("Issue", issue).WithError(err).Error("Failed to ensure GitHub link on Jira issue")
				}
			}(issue)
		}

		if err := updateComment(e, referencedIssues.UnsortedList(), jc.JiraURL(), gc); err != nil {
			errs = append(errs, fmt.Errorf("failed to update comment: %w", err))
		}
		wg.Wait()
		if len(errs) != 0 {
			return utilerrors.NewAggregate(errs)
		}
	}

	if e.key == "" || !strings.HasPrefix(e.key, "OCPBUGS-") {
		return nil
	}

	comment := e.comment(gc)
	// check if bug is part of a restricted security level
	if !e.missing {
		bug, err := getBug(jc, e.key, log, comment)
		if err != nil || bug == nil {
			return err
		}
		bugAllowed, err := isBugAllowed(bug, options.AllowedSecurityLevels)
		if err != nil {
			return err
		}
		if !bugAllowed {
			// ignore bugs that are in non-allowed security levels for this repo
			if e.opened || refreshCommandMatch.MatchString(e.body) {
				response := fmt.Sprintf(bugLink+" is in a security level that is not in the allowed security levels for this repo.", e.key, jc.JiraURL(), e.key)
				if len(options.AllowedSecurityLevels) > 0 {
					response += "\nAllowed security levels for this repo are:"
					for _, group := range options.AllowedSecurityLevels {
						response += "\n- " + group
					}
				} else {
					response += " There are no allowed security levels configured for this repo."
				}
				return comment(response)
			}
			return nil
		}
	}
	// merges follow a different pattern from the normal validation
	if e.merged {
		return handleMerge(e, gc, jc, options, log, allRepos)
	}
	// close events follow a different pattern from the normal validation
	if e.closed && !e.merged {
		return handleClose(e, gc, jc, options, log)
	}
	// cherrypicks follow a different pattern than normal validation
	if e.cherrypick {
		return handleCherrypick(e, gc, jc, options, log)
	}

	var needsValidLabel, needsInvalidLabel bool
	var response, severityLabel string
	if e.missing {
		log.WithField("bugMissing", true)
		log.Debug("No bug referenced.")
		needsValidLabel, needsInvalidLabel = false, false
		response = `No Jira bug is referenced in the title of this pull request.
To reference a bug, add 'OCPBUGS-XXX:' to the title of this pull request and request another bug refresh with <code>/jira refresh</code>.`
	} else {
		log = log.WithField("bugKey", e.key)

		bug, err := getBug(jc, e.key, log, comment)
		if err != nil || bug == nil {
			return err
		}

		severity, err := getSimplifiedSeverity(bug)
		if err != nil {
			return err
		}

		severityLabel = getSeverityLabel(severity)

		var dependents []*jira.Issue
		if options.DependentBugStates != nil || options.DependentBugTargetVersions != nil {
			for _, link := range bug.Fields.IssueLinks {
				// identify if bug depends on this link; multiple different types of links may be blocker types; more can be added as they are identified
				dependsOn := false
				dependsOn = dependsOn || (link.InwardIssue != nil && link.Type.Name == "Blocks" && link.Type.Inward == "is blocked by")
				dependsOn = dependsOn || (link.InwardIssue != nil && link.Type.Name == "Cloners" && link.Type.Inward == "is cloned by")
				if !dependsOn {
					continue
				}
				// link may be either an outward or inward issue; depends on the link type
				linkIssue := link.InwardIssue
				if linkIssue == nil {
					linkIssue = link.OutwardIssue
				}
				// the issue in the link is very trimmed down; get full link for dependent list
				dependent, err := jc.GetIssue(linkIssue.Key)
				if err != nil {
					return comment(formatError(fmt.Sprintf("searching for dependent bug %s", linkIssue.Key), jc.JiraURL(), e.key, err))
				}
				dependents = append(dependents, dependent)
			}
		}

		valid, validationsRun, why := validateBug(bug, dependents, options, jc.JiraURL())
		needsValidLabel, needsInvalidLabel = valid, !valid
		if valid {
			log.Debug("Valid bug found.")
			response = fmt.Sprintf(`This pull request references `+bugLink+`, which is valid.`, e.key, jc.JiraURL(), e.key)
			// if configured, move the bug to the new state
			if options.StateAfterValidation != nil {
				if options.StateAfterValidation.Status != "" && (bug.Fields.Status == nil || options.StateAfterValidation.Status != bug.Fields.Status.Name) {
					if err := jc.UpdateStatus(bug.ID, options.StateAfterValidation.Status); err != nil {
						log.WithError(err).Warn("Unexpected error updating jira issue.")
						return comment(formatError(fmt.Sprintf("updating to the %s state", options.StateAfterValidation.Status), jc.JiraURL(), e.key, err))
					}
					if options.StateAfterValidation.Resolution != "" && (bug.Fields.Resolution == nil || options.StateAfterValidation.Resolution != bug.Fields.Resolution.Name) {
						updateIssue := jira.Issue{ID: bug.ID, Fields: &jira.IssueFields{Resolution: &jira.Resolution{Name: options.StateAfterValidation.Resolution}}}
						if _, err := jc.UpdateIssue(&updateIssue); err != nil {
							log.WithError(err).Warn("Unexpected error updating jira issue.")
							return comment(formatError(fmt.Sprintf("updating to the %s resolution", options.StateAfterValidation.Resolution), jc.JiraURL(), e.key, err))
						}
					}
					response += fmt.Sprintf(" The bug has been moved to the %s state.", options.StateAfterValidation)
				}
			}
			if options.AddExternalLink != nil && *options.AddExternalLink {
				changed, err := upsertGitHubLinkToIssue(log, bug.ID, jc, e)
				if err != nil {
					log.WithError(err).Warn("Unexpected error adding external tracker bug to Jira bug.")
					return comment(formatError("adding this pull request to the external tracker bugs", jc.JiraURL(), e.key, err))
				}
				if changed {
					response += " The bug has been updated to refer to the pull request using the external bug tracker."
				}
			}

			response += "\n\n<details>"
			if len(validationsRun) == 0 {
				response += "<summary>No validations were run on this bug</summary>"
			} else {
				response += fmt.Sprintf("<summary>%d validation(s) were run on this bug</summary>\n", len(validationsRun))
			}
			for _, validation := range validationsRun {
				response += fmt.Sprint("\n* ", validation)
			}
			response += "</details>"

			qaContactDetail, err := jc.GetIssueQaContact(bug)
			if err != nil {
				return comment(formatError("processing qa contact information for the bug", jc.JiraURL(), e.key, err))
			}
			if qaContactDetail == nil {
				if e.cc {
					response += fmt.Sprintf(bugLink+" does not have a QA contact, skipping assignment", e.key, jc.JiraURL(), e.key)
				}
			} else if qaContactDetail.EmailAddress == "" {
				if e.cc {
					response += fmt.Sprintf("QA contact for "+bugLink+" does not have a listed email, skipping assignment", e.key, jc.JiraURL(), e.key)
				}
			} else {
				query := &emailToLoginQuery{}
				email := qaContactDetail.EmailAddress
				queryVars := map[string]interface{}{
					"email": githubql.String(email),
				}
				err := gc.Query(context.Background(), query, queryVars)
				if err != nil {
					log.WithError(err).Error("Failed to run graphql github query")
					return comment(formatError(fmt.Sprintf("querying GitHub for users with public email (%s)", email), jc.JiraURL(), e.key, err))
				}
				response += fmt.Sprint("\n\n", processQuery(query, email, log))
			}
		} else {
			log.Debug("Invalid bug found.")
			var formattedReasons string
			for _, reason := range why {
				formattedReasons += fmt.Sprintf(" - %s\n", reason)
			}
			response = fmt.Sprintf(`This pull request references `+bugLink+`, which is invalid:
%s
Comment <code>/jira refresh</code> to re-evaluate validity if changes to the Jira bug are made, or edit the title of this pull request to link to a different bug.`, e.key, jc.JiraURL(), e.key, formattedReasons)
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
	var severityLabelToRemove string
	for _, l := range currentLabels {
		if l.Name == labels.JiraValidBug {
			hasValidLabel = true
		}
		if l.Name == labels.JiraInvalidBug {
			hasInvalidLabel = true
		}
		if l.Name == labels.JiraSeverityCritical ||
			l.Name == labels.JiraSeverityImportant ||
			l.Name == labels.JiraSeverityModerate ||
			l.Name == labels.JiraSeverityLow ||
			l.Name == labels.JiraSeverityInformational {
			severityLabelToRemove = l.Name
		}
	}

	if severityLabelToRemove != "" && severityLabel != severityLabelToRemove {
		if err := gc.RemoveLabel(e.org, e.repo, e.number, severityLabelToRemove); err != nil {
			log.WithError(err).Error("Failed to remove severity bug label.")
		}
	}
	if severityLabel != "" && severityLabel != severityLabelToRemove {
		if err := gc.AddLabel(e.org, e.repo, e.number, severityLabel); err != nil {
			log.WithError(err).Error("Failed to add severity bug label.")
		}
	}

	if hasValidLabel && !needsValidLabel {
		humanLabelled, err := gc.WasLabelAddedByHuman(e.org, e.repo, e.number, labels.JiraValidBug)
		if err != nil {
			// Return rather than potentially doing the wrong thing. The user can re-trigger us.
			return fmt.Errorf("failed to check if %s label was added by a human: %w", labels.JiraValidBug, err)
		}
		if humanLabelled {
			// This will make us remove the invalid label if it exists but saves us another check if it was
			// added by a human. It is reasonable to assume that it should be absent if the valid label was
			// manually added.
			needsInvalidLabel = false
			needsValidLabel = true
			response += fmt.Sprintf("\n\nRetaining the %s label as it was manually added.", labels.JiraValidBug)
		}
	}

	if needsValidLabel && !hasValidLabel {
		if err := gc.AddLabel(e.org, e.repo, e.number, labels.JiraValidBug); err != nil {
			log.WithError(err).Error("Failed to add valid bug label.")
		}
	} else if !needsValidLabel && hasValidLabel {
		if err := gc.RemoveLabel(e.org, e.repo, e.number, labels.JiraValidBug); err != nil {
			log.WithError(err).Error("Failed to remove valid bug label.")
		}
	}

	if needsInvalidLabel && !hasInvalidLabel {
		if err := gc.AddLabel(e.org, e.repo, e.number, labels.JiraInvalidBug); err != nil {
			log.WithError(err).Error("Failed to add invalid bug label.")
		}
	} else if !needsInvalidLabel && hasInvalidLabel {
		if err := gc.RemoveLabel(e.org, e.repo, e.number, labels.JiraInvalidBug); err != nil {
			log.WithError(err).Error("Failed to remove invalid bug label.")
		}
	}

	return comment(response)
}

// getSimplifiedSeverity retrieves the severity of the issue and trims the image tags that precede
// the name of the severity, which are a nuisance for automation
func getSimplifiedSeverity(issue *jira.Issue) (string, error) {
	severity, err := jiraclient.GetIssueSeverity(issue)
	if err != nil {
		return "", fmt.Errorf("Failed to get severity of issue %s", issue.Key)
	}
	if severity == nil {
		return "unset", nil
	}
	// the values of the severity fields in redhat jira have an image before them
	// (ex: <img alt=\"\" src=\"/images/icons/priorities/medium.svg\" width=\"16\" height=\"16\"> Medium)
	// we need to trim that off before returning. There is always a space between the image and the text,
	// so we can split on spaces and take the last element
	splitSeverity := strings.Split(severity.Value, " ")
	return splitSeverity[len(splitSeverity)-1], nil
}

func updateComment(e event, validIssues []string, jiraBaseURL string, ghc githubClient) error {
	withLinks := insertLinksIntoComment(e.body, validIssues, jiraBaseURL)
	if withLinks == e.body {
		return nil
	}
	if e.commentID != nil {
		return ghc.EditComment(e.org, e.repo, *e.commentID, withLinks)
	}

	issue, err := ghc.GetIssue(e.org, e.repo, e.number)
	if err != nil {
		return fmt.Errorf("failed to get issue %s/%s#%d: %w", e.org, e.repo, e.number, err)
	}

	// Check for the diff on the issues body in case the event didn't have a commentID but did not originate
	// in issue creation, e.G. PRReviewEvent
	if withLinks := insertLinksIntoComment(issue.Body, validIssues, jiraBaseURL); withLinks != issue.Body {
		issue.Body = withLinks
		_, err := ghc.EditIssue(e.org, e.repo, e.number, issue)
		return err
	}

	return nil
}

type line struct {
	content   string
	replacing bool
}

func getLines(text string) []line {
	var lines []line
	rawLines := strings.Split(text, "\n")
	var prefixCount int
	for _, rawLine := range rawLines {
		if strings.HasPrefix(rawLine, "```") {
			prefixCount++
		}
		l := line{content: rawLine, replacing: true}

		// Literal codeblocks
		if strings.HasPrefix(rawLine, "    ") {
			l.replacing = false
		}
		if prefixCount%2 == 1 {
			l.replacing = false
		}
		lines = append(lines, l)
	}
	return lines
}

func insertLinksIntoComment(body string, issueNames []string, jiraBaseURL string) string {
	var linesWithLinks []string
	lines := getLines(body)
	for _, line := range lines {
		if line.replacing {
			linesWithLinks = append(linesWithLinks, insertLinksIntoLine(line.content, issueNames, jiraBaseURL))
			continue
		}
		linesWithLinks = append(linesWithLinks, line.content)
	}
	return strings.Join(linesWithLinks, "\n")
}

func insertLinksIntoLine(line string, issueNames []string, jiraBaseURL string) string {
	for _, issue := range issueNames {
		replacement := fmt.Sprintf("[%s](%s/browse/%s)", issue, jiraBaseURL, issue)
		line = replaceStringIfNeeded(line, issue, replacement)
	}
	return line
}

// replaceStringIfNeeded replaces a string if it is not prefixed by:
// * `[` which we use as heuristic for "Already replaced",
// * `/` which we use as heuristic for "Part of a link in a previous replacement",
// * ``` (backtick) which we use as heuristic for "Inline code".
// If golang would support back-references in regex replacements, this would have been a lot
func replaceStringIfNeeded(text, old, new string) string {
	if old == "" {
		return text
	}

	var result string

	// Golangs stdlib has no strings.IndexAll, only funcs to get the first
	// or last index for a substring. Definitions/condition/assignments are not
	// in the header of the loop because that makes it completely unreadable.
	var allOldIdx []int
	var startingIdx int
	for {
		idx := strings.Index(text[startingIdx:], old)
		if idx == -1 {
			break
		}
		idx = startingIdx + idx
		// Since we always look for a non-empty string, we know that idx++
		// can not be out of bounds
		allOldIdx = append(allOldIdx, idx)
		startingIdx = idx + 1
	}

	startingIdx = 0
	for _, idx := range allOldIdx {
		result += text[startingIdx:idx]
		if idx == 0 || (text[idx-1] != '[' && text[idx-1] != '/') && text[idx-1] != '`' {
			result += new
		} else {
			result += old
		}
		startingIdx = idx + len(old)
	}
	result += text[startingIdx:]

	return result
}

func prURLFromCommentURL(url string) string {
	newURL := url
	if idx := strings.Index(url, "#"); idx != -1 {
		newURL = newURL[:idx]
	}
	return newURL
}

// upsertGitHubLinkToIssue adds a remote link to the github issue on the jira issue. It returns a bool indicating whether or not the
// remote link changed or was created, and an error.
func upsertGitHubLinkToIssue(log *logrus.Entry, issueID string, jc jiraclient.Client, e event) (bool, error) {
	links, err := jc.GetRemoteLinks(issueID)
	if err != nil {
		return false, fmt.Errorf("failed to get remote links: %w", err)
	}

	url := prURLFromCommentURL(e.htmlUrl)
	title := fmt.Sprintf("%s/%s#%d: %s", e.org, e.repo, e.number, e.title)
	var existingLink *jira.RemoteLink

	// Check if the same link exists already. We consider two links to be the same if the have the same URL.
	// Once it is found we have two possibilities: either it is really equal (just skip the upsert) or it
	// has to be updated (perform an upsert)
	for _, link := range links {
		if link.Object.URL == url {
			if title == link.Object.Title {
				return false, nil
			}
			link := link
			existingLink = &link
			break
		}
	}

	link := &jira.RemoteLink{
		Object: &jira.RemoteLinkObject{
			URL:   url,
			Title: title,
			Icon: &jira.RemoteLinkIcon{
				Url16x16: "https://github.com/favicon.ico",
				Title:    "GitHub",
			},
		},
	}

	if existingLink != nil {
		existingLink.Object = link.Object
		if err := jc.UpdateRemoteLink(issueID, existingLink); err != nil {
			return false, fmt.Errorf("failed to update remote link: %w", err)
		}
		log.Info("Updated jira link")
	} else {
		if _, err := jc.AddRemoteLink(issueID, link); err != nil {
			return false, fmt.Errorf("failed to add remote link: %w", err)
		}
		log.Info("Created jira link")
	}

	return true, nil
}

func filterOutDisabledJiraProjects(candidateNames []string, disabledProjects []string) []string {
	if len(disabledProjects) == 0 {
		return candidateNames
	}

	var result []string
	for _, excludedProject := range disabledProjects {
		for _, candidate := range candidateNames {
			if strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(excludedProject)) {
				continue
			}
			result = append(result, candidate)
		}
	}

	return result
}

// projectCachingJiraClient caches 404 for projects and uses them to introduce
// a fastpath in GetIssue for returning a 404.
type projectCachingJiraClient struct {
	jiraclient.Client
	cache *threadsafeSet
}

func (c *projectCachingJiraClient) GetIssue(id string) (*jira.Issue, error) {
	projectName := strings.ToLower(strings.Split(id, "-")[0])
	if !c.cache.has(projectName) {
		return nil, jiraclient.NewNotFoundError(fmt.Errorf("404 from cache for key %s, project name %s", id, projectName))
	}
	result, err := c.Client.GetIssue(id)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type threadsafeSet struct {
	data sets.String
	lock sync.RWMutex
}

func (s *threadsafeSet) has(projectName string) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.data.Has(projectName)
}

func (s *threadsafeSet) insert(projectName ...string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.data.Insert(projectName...)
}

func (s *threadsafeSet) entryCount() int {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return len(s.data)
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered panic in jira plugin: %v", r)
		}
	}()
	options := pc.PluginConfig.Jira.OptionsForBranch(pre.PullRequest.Base.Repo.Owner.Login, pre.PullRequest.Base.Repo.Name, pre.PullRequest.Base.Ref)
	event, err := digestPR(pc.Logger, pre, options.ValidateByDefault)
	if err != nil {
		return err
	}
	if event != nil {
		return handle(pc.JiraClient, pc.GitHubClient, pc.PluginConfig.Jira.DisabledJiraProjects, options, pc.Logger, *event, pc.Config.AllRepos)
	}
	return nil
}

func getCherryPickMatch(pre github.PullRequestEvent) (bool, int, string, error) {
	cherrypickMatch := cherrypickPRMatch.FindStringSubmatch(pre.PullRequest.Body)
	if cherrypickMatch != nil {
		cherrypickOf, err := strconv.Atoi(cherrypickMatch[1])
		if err != nil {
			// should be impossible based on the regex
			return false, 0, "", fmt.Errorf("Failed to parse cherrypick jira issue - is the regex correct? Err: %w", err)
		}
		return true, cherrypickOf, pre.PullRequest.Base.Ref, nil
	}
	return false, 0, "", nil
}

// digestPR determines if any action is necessary and creates the objects for handle() if it is
func digestPR(log *logrus.Entry, pre github.PullRequestEvent, validateByDefault *bool) (*event, error) {
	// These are the only actions indicating the PR title may have changed or that the PR merged or was closed
	if pre.Action != github.PullRequestActionOpened &&
		pre.Action != github.PullRequestActionReopened &&
		pre.Action != github.PullRequestActionEdited &&
		pre.Action != github.PullRequestActionClosed {
		return nil, nil
	}

	var (
		org     = pre.PullRequest.Base.Repo.Owner.Login
		repo    = pre.PullRequest.Base.Repo.Name
		baseRef = pre.PullRequest.Base.Ref
		number  = pre.PullRequest.Number
		title   = pre.PullRequest.Title
		body    = pre.PullRequest.Body
	)

	e := &event{org: org, repo: repo, baseRef: baseRef, number: number, merged: pre.PullRequest.Merged, closed: pre.Action == github.PullRequestActionClosed, opened: pre.Action == github.PullRequestActionOpened, state: pre.PullRequest.State, body: body, title: title, htmlUrl: pre.PullRequest.HTMLURL, login: pre.PullRequest.User.Login}
	// Make sure the PR title is referencing a bug
	var err error
	e.key, e.missing, err = bugKeyFromTitle(title)
	// in the case that the title used to reference a bug and no longer does we
	// want to handle this to remove labels
	if err != nil {
		log.WithError(err).Debug("Failed to get bug ID from title")
		return nil, err
	}

	// Check if PR is a cherrypick
	cherrypick, cherrypickFromPRNum, cherrypickTo, err := getCherryPickMatch(pre)
	if err != nil {
		log.WithError(err).Debug("Failed to identify if PR is a cherrypick")
		return nil, err
	} else if cherrypick {
		if pre.Action == github.PullRequestActionOpened {
			e.cherrypick = true
			e.cherrypickFromPRNum = cherrypickFromPRNum
			e.cherrypickTo = cherrypickTo
			return e, nil
		}
	}

	if e.closed && !e.merged {
		// if the PR was closed, we do not need to check for any other
		// conditions like cherry-picks or title edits and can just
		// handle it
		return e, nil
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
	prevId, missing, err := bugKeyFromTitle(changes.Title.From)
	if missing {
		// title did not previously reference a bug
		return intermediate, nil
	} else if err != nil {
		// should be impossible based on the regex, ignore err as this is best-effort
		log.WithError(err).Debug("Failed get previous bug ID")
		return intermediate, nil
	}

	// if the referenced bug has not changed in the update, ignore it
	if prevId == e.key {
		logrus.Debugf("Referenced OCPBUGS story (%s) has not changed, not handling event.", e.key)
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
	// Make sure they are requesting a valid command
	var cc bool
	switch {
	case refreshCommandMatch.MatchString(gce.Body):
		// continue without updating bool values
	case qaReviewCommandMatch.MatchString(gce.Body):
		cc = true
	default:
		return nil, nil
	}
	var (
		org    = gce.Repo.Owner.Login
		repo   = gce.Repo.Name
		number = gce.Number
	)

	// We don't support linking issues to Bugs
	if !gce.IsPR {
		log.Debug("Jira bug command requested on an issue, ignoring")
		return nil, gc.CreateComment(org, repo, number, plugins.FormatResponseRaw(gce.Body, gce.HTMLURL, gce.User.Login, `Jira bug referencing is only supported for Pull Requests, not issues.`))
	}

	// Make sure the PR title is referencing a bug
	pr, err := gc.GetPullRequest(org, repo, number)
	if err != nil {
		return nil, err
	}

	e := &event{org: org, repo: repo, baseRef: pr.Base.Ref, number: number, merged: pr.Merged, state: pr.State, body: gce.Body, title: gce.IssueTitle, htmlUrl: gce.HTMLURL, login: gce.User.Login, cc: cc, isComment: true, commentID: gce.CommentID}
	e.key, e.missing, err = bugKeyFromTitle(pr.Title)
	if err != nil {
		// should be impossible based on the regex
		log.WithError(err).Debug("Failed to get Jira bug ID from PR title")
		return nil, err
	}

	return e, nil
}

type event struct {
	org, repo, baseRef              string
	number                          int
	key                             string
	missing, merged, closed, opened bool
	isComment                       bool
	commentID                       *int
	state                           string
	body, title, htmlUrl, login     string
	cc                              bool
	cherrypick                      bool
	cherrypickFromPRNum             int
	cherrypickTo                    string
}

func (e *event) comment(gc githubClient) func(body string) error {
	return func(body string) error {
		return gc.CreateComment(e.org, e.repo, e.number, plugins.FormatResponseRaw(e.body, e.htmlUrl, e.login, body))
	}
}

type queryUser struct {
	Login githubql.String
}

type queryNode struct {
	User queryUser `graphql:"... on User"`
}

type queryEdge struct {
	Node queryNode
}

type querySearch struct {
	Edges []queryEdge
}

/* emailToLoginQuery is a graphql query struct that should result in this graphql query:
   {
     search(type: USER, query: "email", first: 5) {
       edges {
         node {
           ... on User {
             login
           }
         }
       }
     }
   }
*/
type emailToLoginQuery struct {
	Search querySearch `graphql:"search(type:USER query:$email first:5)"`
}

// processQueryResult generates a response based on a populated emailToLoginQuery
func processQuery(query *emailToLoginQuery, email string, log *logrus.Entry) string {
	switch len(query.Search.Edges) {
	case 0:
		return fmt.Sprintf("No GitHub users were found matching the public email listed for the QA contact in Jira (%s), skipping review request.", email)
	case 1:
		return fmt.Sprintf("Requesting review from QA contact:\n/cc @%s", query.Search.Edges[0].Node.User.Login)
	default:
		response := fmt.Sprintf("Multiple GitHub users were found matching the public email listed for the QA contact in Jira (%s), skipping review request. List of users with matching email:", email)
		for _, edge := range query.Search.Edges {
			response += fmt.Sprintf("\n\t- %s", edge.Node.User.Login)
		}
		return response
	}
}

func getSeverityLabel(severity string) string {
	switch severity {
	case criticalSeverity:
		return labels.JiraSeverityCritical
	case importantSeverity:
		return labels.JiraSeverityImportant
	case moderateSeverity:
		return labels.JiraSeverityModerate
	case lowSeverity:
		return labels.JiraSeverityLow
	case informationalSeverity:
		return labels.JiraSeverityInformational
	}
	//If we don't understand the severity, don't set it but don't error.
	return ""
}

func bugMatchesStates(bug *jira.Issue, states []plugins.JiraBugState) bool {
	if bug == nil {
		return false
	}
	for _, state := range states {
		if ((state.Status != "" && state.Status == bug.Fields.Status.Name) || state.Status == "") &&
			((state.Resolution != "" && state.Resolution == bug.Fields.Resolution.Name) || state.Resolution == "") {
			return true
		}
	}
	return false
}

func prettyStates(statuses []plugins.JiraBugState) []string {
	pretty := make([]string, 0, len(statuses))
	for _, status := range statuses {
		pretty = append(pretty, jiraclient.PrettyStatus(status.Status, status.Resolution))
	}
	return pretty
}

// validateBug determines if the bug matches the options and returns a description of why not
func validateBug(bug *jira.Issue, dependents []*jira.Issue, options plugins.JiraBranchOptions, endpoint string) (bool, []string, []string) {
	valid := true
	var errors []string
	var validations []string
	if options.IsOpen != nil && (bug.Fields == nil || bug.Fields.Status == nil || *options.IsOpen != (bug.Fields.Status.Name != jiraclient.StatusClosed)) {
		valid = false
		not := ""
		was := "isn't"
		if !*options.IsOpen {
			not = "not "
			was = "is"
		}
		errors = append(errors, fmt.Sprintf("expected the bug to %sbe open, but it %s", not, was))
	} else if options.IsOpen != nil {
		expected := "open"
		if !*options.IsOpen {
			expected = "not open"
		}
		was := "isn't"
		if bug.Fields.Status.Name != jiraclient.StatusClosed {
			was = "is"
		}
		validations = append(validations, fmt.Sprintf("bug %s open, matching expected state (%s)", was, expected))
	}

	if options.TargetVersion != nil {
		targetVersion, err := jiraclient.GetIssueTargetVersion(bug)
		if err != nil {
			valid = false
			errors = append(errors, fmt.Sprintf("failed to get target version for bug: %v", err))
		} else {
			if len(targetVersion) == 0 {
				valid = false
				errors = append(errors, fmt.Sprintf("expected the bug to target the %q version, but no target version was set", *options.TargetVersion))
			} else if len(targetVersion) > 1 {
				valid = false
				errors = append(errors, fmt.Sprintf("expected the bug to target only the %q version, but multiple target versions were set", *options.TargetVersion))
			} else if *options.TargetVersion != targetVersion[0].Name {
				valid = false
				errors = append(errors, fmt.Sprintf("expected the bug to target the %q version, but it targets %q instead", *options.TargetVersion, targetVersion[0].Name))
			} else {
				validations = append(validations, fmt.Sprintf("bug target version (%s) matches configured target version for branch (%s)", targetVersion[0].Name, *options.TargetVersion))
			}
		}
	}

	if options.ValidStates != nil {
		var allowed []plugins.JiraBugState
		allowed = append(allowed, *options.ValidStates...)
		if options.StateAfterValidation != nil {
			allowed = append(allowed, *options.StateAfterValidation)
		}
		var status, resolution string
		if bug.Fields.Status != nil {
			status = bug.Fields.Status.Name
		}
		if bug.Fields.Resolution != nil {
			resolution = bug.Fields.Resolution.Name
		}
		if !bugMatchesStates(bug, allowed) {
			valid = false
			errors = append(errors, fmt.Sprintf("expected the bug to be in one of the following states: %s, but it is %s instead", strings.Join(prettyStates(allowed), ", "), jiraclient.PrettyStatus(status, resolution)))
		} else {
			validations = append(validations, fmt.Sprintf("bug is in the state %s, which is one of the valid states (%s)", jiraclient.PrettyStatus(status, resolution), strings.Join(prettyStates(allowed), ", ")))
		}
	}

	if options.DependentBugStates != nil {
		for _, bug := range dependents {
			var status, resolution string
			if bug.Fields.Status != nil {
				status = bug.Fields.Status.Name
			}
			if bug.Fields.Resolution != nil {
				resolution = bug.Fields.Resolution.Name
			}
			if !bugMatchesStates(bug, *options.DependentBugStates) {
				valid = false
				expected := strings.Join(prettyStates(*options.DependentBugStates), ", ")
				actual := jiraclient.PrettyStatus(status, resolution)
				errors = append(errors, fmt.Sprintf("expected dependent "+bugLink+" to be in one of the following states: %s, but it is %s instead", bug.Key, endpoint, bug.Key, expected, actual))
			} else {
				validations = append(validations, fmt.Sprintf("dependent bug "+bugLink+" is in the state %s, which is one of the valid states (%s)", bug.Key, endpoint, bug.Key, jiraclient.PrettyStatus(status, resolution), strings.Join(prettyStates(*options.DependentBugStates), ", ")))
			}
		}
	}

	if options.DependentBugTargetVersions != nil {
		for _, bug := range dependents {
			targetVersion, err := jiraclient.GetIssueTargetVersion(bug)
			if err != nil {
				valid = false
				errors = append(errors, fmt.Sprintf("failed to get target version for bug: %v", err))
			} else {
				if len(targetVersion) == 0 {
					valid = false
					errors = append(errors, fmt.Sprintf("expected dependent "+bugLink+" to target a version in %s, but no target version was set", bug.Key, endpoint, bug.Key, strings.Join(*options.DependentBugTargetVersions, ", ")))
				} else if len(targetVersion) > 1 {
					valid = false
					errors = append(errors, fmt.Sprintf("expected dependent "+bugLink+" to target a version in %s, but it has multiple target versions", bug.Key, endpoint, bug.Key, strings.Join(*options.DependentBugTargetVersions, ", ")))
				} else if sets.NewString(*options.DependentBugTargetVersions...).Has(targetVersion[0].Name) {
					validations = append(validations, fmt.Sprintf("dependent "+bugLink+" targets the %q version, which is one of the valid target versions: %s", bug.Key, endpoint, bug.Key, targetVersion[0].Name, strings.Join(*options.DependentBugTargetVersions, ", ")))
				} else {
					valid = false
					errors = append(errors, fmt.Sprintf("expected dependent "+bugLink+" to target a version in %s, but it targets %q instead", bug.Key, endpoint, bug.Key, strings.Join(*options.DependentBugTargetVersions, ", "), targetVersion[0].Name))
				}
			}
		}
	}

	if len(dependents) == 0 {
		switch {
		case options.DependentBugStates != nil && options.DependentBugTargetVersions != nil:
			valid = false
			expected := strings.Join(prettyStates(*options.DependentBugStates), ", ")
			errors = append(errors, fmt.Sprintf("expected "+bugLink+" to depend on a bug targeting a version in %s and in one of the following states: %s, but no dependents were found", bug.Key, endpoint, bug.Key, strings.Join(*options.DependentBugTargetVersions, ", "), expected))
		case options.DependentBugStates != nil:
			valid = false
			expected := strings.Join(prettyStates(*options.DependentBugStates), ", ")
			errors = append(errors, fmt.Sprintf("expected "+bugLink+" to depend on a bug in one of the following states: %s, but no dependents were found", bug.Key, endpoint, bug.Key, expected))
		case options.DependentBugTargetVersions != nil:
			valid = false
			errors = append(errors, fmt.Sprintf("expected "+bugLink+" to depend on a bug targeting a version in %s, but no dependents were found", bug.Key, endpoint, bug.Key, strings.Join(*options.DependentBugTargetVersions, ", ")))
		default:
		}
	} else {
		validations = append(validations, "bug has dependents")
	}

	return valid, validations, errors
}

type prParts struct {
	Org  string
	Repo string
	Num  int
}

func handleMerge(e event, gc githubClient, jc jiraclient.Client, options plugins.JiraBranchOptions, log *logrus.Entry, allRepos sets.String) error {
	comment := e.comment(gc)

	if options.StateAfterMerge == nil {
		return nil
	}
	if e.missing {
		return nil
	}
	bug, err := getBug(jc, e.key, log, comment)
	if err != nil || bug == nil {
		return err
	}
	if options.ValidStates != nil || options.StateAfterValidation != nil {
		// we should only migrate if we can be fairly certain that the bug
		// is not in a state that required human intervention to get to.
		// For instance, if a bug is closed after a PR merges it should not
		// be possible for /jira refresh to move it back to the post-merge
		// state.
		var allowed []plugins.JiraBugState
		if options.ValidStates != nil {
			allowed = append(allowed, *options.ValidStates...)
		}

		if options.StateAfterValidation != nil {
			allowed = append(allowed, *options.StateAfterValidation)
		}
		if !bugMatchesStates(bug, allowed) {
			return comment(fmt.Sprintf(bugLink+" is in an unrecognized state (%s) and will not be moved to the %s state.", e.key, jc.JiraURL(), e.key, bug.Fields.Status.Name, options.StateAfterMerge))
		}
	}

	links, err := jc.GetRemoteLinks(bug.ID)
	if err != nil {
		log.WithError(err).Warn("Unexpected error listing external tracker bugs for Jira bug.")
		return comment(formatError("searching for external tracker bugs", jc.JiraURL(), e.key, err))
	}
	shouldMigrate := true
	var mergedPRs []prParts
	unmergedPrStates := map[prParts]string{}
	for _, link := range links {
		identifier := strings.TrimPrefix(link.Object.URL, "https://github.com/")
		parts := strings.Split(identifier, "/")
		if len(parts) >= 3 && parts[2] != "pull" {
			// this is not a github link
			continue
		}
		if len(parts) != 4 && !(len(parts) == 5 && (parts[4] == "" || parts[4] == "files")) && !(len(parts) == 6 && (parts[4] == "files" && parts[5] == "")) {
			log.WithError(err).Warn("Unexpected error splitting github URL for Jira external link.")
			return comment(formatError(fmt.Sprintf("invalid pull identifier with %d parts: %q", len(parts), identifier), jc.JiraURL(), e.key, err))
		}
		number, err := strconv.Atoi(parts[3])
		if err != nil {
			log.WithError(err).Warn("Unexpected error splitting github URL for Jira external link.")
			return comment(formatError(fmt.Sprintf("invalid pull identifier: could not parse %s as number", parts[3]), jc.JiraURL(), e.key, err))
		}
		item := prParts{
			Org:  parts[0],
			Repo: parts[1],
			Num:  number,
		}
		var merged bool
		var state string
		if e.org == item.Org && e.repo == item.Repo && e.number == item.Num {
			merged = e.merged
			state = e.state
		} else {
			// This could be literally anything, only process PRs in repos that are mentioned in our config, otherwise this will potentially
			// fail.
			if !allRepos.Has(item.Org + "/" + item.Repo) {
				logrus.WithField("pr", item.Org+"/"+item.Repo+"#"+strconv.Itoa(item.Num)).Debug("Not processing PR from third-party repo")
				continue
			}
			pr, err := gc.GetPullRequest(item.Org, item.Repo, item.Num)
			if err != nil {
				log.WithError(err).Warn("Unexpected error checking merge state of related pull request.")
				return comment(formatError(fmt.Sprintf("checking the state of a related pull request at https://github.com/%s/%s/pull/%d", item.Org, item.Repo, item.Num), jc.JiraURL(), e.key, err))
			}
			merged = pr.Merged
			state = pr.State
		}
		if merged {
			mergedPRs = append(mergedPRs, item)
		} else {
			unmergedPrStates[item] = state
		}
		// only update Jira bug status if all PRs have merged
		shouldMigrate = shouldMigrate && merged
		if !shouldMigrate {
			// we could give more complete feedback to the user by checking all PRs
			// but we save tokens by exiting when we find an unmerged one, so we
			// prefer to do that
			break
		}
	}

	link := func(pr prParts) string {
		return fmt.Sprintf("[%s/%s#%d](https://github.com/%s/%s/pull/%d)", pr.Org, pr.Repo, pr.Num, pr.Org, pr.Repo, pr.Num)
	}

	mergedMessage := func(statement string) string {
		var links []string
		for _, bug := range mergedPRs {
			links = append(links, fmt.Sprintf(" * %s", link(bug)))
		}
		return fmt.Sprintf(`%s pull requests linked via external trackers have merged:
%s

`, statement, strings.Join(links, "\n"))
	}

	var statements []string
	for bug, state := range unmergedPrStates {
		statements = append(statements, fmt.Sprintf(" * %s is %s", link(bug), state))
	}
	unmergedMessage := fmt.Sprintf(`The following pull requests linked via external trackers have not merged:
%s

These pull request must merge or be unlinked from the Jira bug in order for it to move to the next state. Once unlinked, request a bug refresh with <code>/jira refresh</code>.

`, strings.Join(statements, "\n"))

	outcomeMessage := func(action string) string {
		return fmt.Sprintf(bugLink+" has %sbeen moved to the %s state.", e.key, jc.JiraURL(), e.key, action, options.StateAfterMerge)
	}

	if shouldMigrate {
		if options.StateAfterMerge != nil {
			if options.StateAfterMerge.Status != "" && (bug.Fields.Status == nil || options.StateAfterMerge.Status != bug.Fields.Status.Name) {
				if err := jc.UpdateStatus(e.key, options.StateAfterMerge.Status); err != nil {
					log.WithError(err).Warn("Unexpected error updating jira issue.")
					return comment(formatError(fmt.Sprintf("updating to the %s state", options.StateAfterMerge.Status), jc.JiraURL(), e.key, err))
				}
				if options.StateAfterMerge.Resolution != "" && (bug.Fields.Resolution == nil || options.StateAfterMerge.Resolution != bug.Fields.Resolution.Name) {
					updateIssue := jira.Issue{ID: bug.ID, Fields: &jira.IssueFields{Resolution: &jira.Resolution{Name: options.StateAfterMerge.Resolution}}}
					if _, err := jc.UpdateIssue(&updateIssue); err != nil {
						log.WithError(err).Warn("Unexpected error updating jira issue.")
						return comment(formatError(fmt.Sprintf("updating to the %s resolution", options.StateAfterMerge.Resolution), jc.JiraURL(), e.key, err))
					}
				}
			}
		}
		return comment(fmt.Sprintf("%s%s", mergedMessage("All"), outcomeMessage("")))
	}
	return comment(fmt.Sprintf("%s%s%s", mergedMessage("Some"), unmergedMessage, outcomeMessage("not ")))
}

func identifyClones(issue *jira.Issue) []*jira.Issue {
	var clones []*jira.Issue
	for _, link := range issue.Fields.IssueLinks {
		// the inward issue of the Cloners type is always the clone of the provided; if it is unset (nil), then
		// the issue being linked is being cloned by the provided issue
		if link.Type.Name == "Cloners" && link.InwardIssue != nil {
			clones = append(clones, link.InwardIssue)
		}
	}
	return clones
}

func handleCherrypick(e event, gc githubClient, jc jiraclient.Client, options plugins.JiraBranchOptions, log *logrus.Entry) error {
	comment := e.comment(gc)
	// get the info for the PR being cherrypicked from
	pr, err := gc.GetPullRequest(e.org, e.repo, e.cherrypickFromPRNum)
	if err != nil {
		log.WithError(err).Warn("Unexpected error getting title of pull request being cherrypicked from.")
		return comment(fmt.Sprintf("Error creating a cherry-pick bug in Jira: failed to check the state of cherrypicked pull request at https://github.com/%s/%s/pull/%d: %v.\nPlease contact an administrator to resolve this issue, then request a bug refresh with <code>/jira refresh</code>.", e.org, e.repo, e.cherrypickFromPRNum, err))
	}
	// Attempt to identify bug from PR title
	bugKey, bugMissing, err := bugKeyFromTitle(pr.Title)
	if err != nil {
		// should be impossible based on the regex
		log.WithError(err).Debugf("Failed to get bug ID from PR title \"%s\"", pr.Title)
		return comment(fmt.Sprintf("Error creating a cherry-pick bug in Jira: could not get bug ID from PR title \"%s\": %v", pr.Title, err))
	} else if bugMissing {
		log.Debugf("Parent PR %d doesn't have associated bug; not creating cherrypicked bug", pr.Number)
		// if there is no jira bug, we should simply ignore this PR
		return nil
	}
	// Since getBug generates a comment itself, we have to add a prefix explaining that this was a cherrypick attempt to the comment
	commentWithPrefix := func(body string) error {
		return comment(fmt.Sprintf("Failed to create a cherry-pick bug in Jira: %s", body))
	}
	bug, err := getBug(jc, bugKey, log, commentWithPrefix)
	if err != nil || bug == nil {
		return err
	}
	allowed, err := isBugAllowed(bug, options.AllowedSecurityLevels)
	if err != nil {
		return fmt.Errorf("failed to check is issue is in allowed security level: %w", err)
	}
	if !allowed {
		// ignore bugs that are in non-allowed groups for this repo
		return nil
	}
	clones := identifyClones(bug)
	oldLink := fmt.Sprintf(bugLink, bugKey, jc.JiraURL(), bugKey)
	if options.TargetVersion == nil {
		return comment(fmt.Sprintf("Could not make automatic cherrypick of %s for this PR as the target version is not set for this branch in the jira plugin config. Running refresh:\n/jira refresh", oldLink))
	}
	targetRelease := *options.TargetVersion
	for _, baseClone := range clones {
		// get full issue struct
		clone, err := jc.GetIssue(baseClone.Key)
		if err != nil {
			return fmt.Errorf("failed to get %s, which is a clone of %s: %w", baseClone.Key, bug.Key, err)
		}
		targetVersion, err := jiraclient.GetIssueTargetVersion(clone)
		if err != nil {
			return comment(formatError(fmt.Sprintf("getting the target version for clone %s", clone.Key), jc.JiraURL(), bug.Key, err))
		}
		if len(targetVersion) == 1 && targetVersion[0].Name == targetRelease {
			newTitle := strings.Replace(e.title, bugKey, clone.Key, 1)
			return comment(fmt.Sprintf("Detected clone of %s with correct target version. Retitling PR to link to clone:\n/retitle %s", oldLink, newTitle))
		}
	}
	clone, err := jc.CloneIssue(bug)
	if err != nil {
		log.WithError(err).Debugf("Failed to clone bug %s", bugKey)
		return comment(formatError("cloning bug for cherrypick", jc.JiraURL(), bug.Key, err))
	}
	cloneLink := fmt.Sprintf(bugLink, clone.Key, jc.JiraURL(), clone.Key)
	// Update the version of the bug to the target release
	update := jira.Issue{
		ID: clone.ID,
		Fields: &jira.IssueFields{
			Unknowns: tcontainer.MarshalMap{
				"customfield_12319940": []*jira.Version{{Name: targetRelease}},
			},
		},
	}
	_, err = jc.UpdateIssue(&update)
	if err != nil {
		log.WithError(err).Debugf("Unable to update target version and dependencies for bug %s", clone.Key)
		return comment(formatError(fmt.Sprintf("updating cherry-pick bug in Jira: Created cherrypick %s, but encountered error updating target version", cloneLink), jc.JiraURL(), clone.Key, err))
	}
	// Replace old bugID in title with new cloneID
	newTitle := strings.ReplaceAll(e.title, bugKey, clone.Key)
	response := fmt.Sprintf("%s has been cloned as %s. Retitling PR to link against new bug.\n/retitle %s", oldLink, cloneLink, newTitle)
	return comment(response)
}

func bugKeyFromTitle(title string) (string, bool, error) {
	mat := titleMatch.FindStringSubmatch(title)
	if mat == nil {
		return "", true, nil
	}
	return strings.TrimSuffix(mat[0], ":"), false, nil
}

func getBug(jc jiraclient.Client, bugKey string, log *logrus.Entry, comment func(string) error) (*jira.Issue, error) {
	bug, err := jc.GetIssue(bugKey)
	if err != nil && !jiraclient.IsNotFound(err) {
		log.WithError(err).Warn("Unexpected error searching for Jira bug.")
		return nil, comment(formatError("searching", jc.JiraURL(), bugKey, err))
	}
	if jiraclient.IsNotFound(err) || bug == nil {
		log.Debug("No bug found.")
		return nil, comment(fmt.Sprintf(`No Jira issue with key %s exists in the tracker at %s.
Once a valid bug is referenced in the title of this pull request, request a bug refresh with <code>/jira refresh</code>.`,
			bugKey, jc.JiraURL()))
	}
	return bug, nil
}

func formatError(action, endpoint, bugKey string, err error) string {
	knownErrors := map[string]string{
		// TODO: Most of this code is copied from the bugzilla client. If Jira rate limits us the same way, this could come in handy. We will keep this for now in case it is needed
		//"There was an error reported for a GitHub REST call": "The Bugzilla server failed to load data from GitHub when creating the bug. This is usually caused by rate-limiting, please try again later.",
	}
	var applicable []string
	for key, value := range knownErrors {
		if strings.Contains(err.Error(), key) {
			applicable = append(applicable, value)

		}
	}
	digest := "No known errors were detected, please see the full error message for details."
	if len(applicable) > 0 {
		digest = "We were able to detect the following conditions from the error:\n\n"
		for _, item := range applicable {
			digest = fmt.Sprintf("%s- %s\n", digest, item)
		}
	}
	return fmt.Sprintf(`An error was encountered %s for bug %s on the Jira server at %s. %s

<details><summary>Full error message.</summary>

<code>
%v
</code>

</details>

Please contact an administrator to resolve this issue, then request a bug refresh with <code>/jira refresh</code>.`,
		action, bugKey, endpoint, digest, err)
}

var PrivateVisibility = jira.CommentVisibility{Type: "group", Value: "Red Hat Employee"}

func handleClose(e event, gc githubClient, jc jiraclient.Client, options plugins.JiraBranchOptions, log *logrus.Entry) error {
	comment := e.comment(gc)
	if e.missing {
		return nil
	}
	if options.AddExternalLink != nil && *options.AddExternalLink {
		response := fmt.Sprintf(`This pull request references `+bugLink+`. The bug has been updated to no longer refer to the pull request using the external bug tracker.`, e.key, jc.JiraURL(), e.key)
		changed, err := jc.DeleteRemoteLinkViaURL(e.key, prURLFromCommentURL(e.htmlUrl))
		if err != nil {
			log.WithError(err).Warn("Unexpected error removing external tracker bug from Jira bug.")
			return comment(formatError("removing this pull request from the external tracker bugs", jc.JiraURL(), e.key, err))
		}
		if options.StateAfterClose != nil {
			issue, err := jc.GetIssue(e.key)
			if err != nil {
				log.WithError(err).Warn("Unexpected error getting Jira issue.")
				return comment(formatError("getting issue", jc.JiraURL(), e.key, err))
			}
			if issue.Fields.Status.Name != "CLOSED" {
				links, err := jc.GetRemoteLinks(issue.ID)
				if err != nil {
					log.WithError(err).Warn("Unexpected error getting remote links for Jira issue.")
					return comment(formatError("getting remote links", jc.JiraURL(), e.key, err))
				}
				if len(links) == 0 {
					bug, err := getBug(jc, e.key, log, comment)
					if err != nil || bug == nil {
						return err
					}
					if options.StateAfterClose.Status != "" && (bug.Fields.Status == nil || options.StateAfterClose.Status != bug.Fields.Status.Name) {
						if err := jc.UpdateStatus(issue.ID, options.StateAfterClose.Status); err != nil {
							log.WithError(err).Warn("Unexpected error updating jira issue.")
							return comment(formatError(fmt.Sprintf("updating to the %s state", options.StateAfterClose.Status), jc.JiraURL(), e.key, err))
						}
						if options.StateAfterClose.Resolution != "" && (bug.Fields.Resolution == nil || options.StateAfterClose.Resolution != bug.Fields.Resolution.Name) {
							updateIssue := jira.Issue{ID: bug.ID, Fields: &jira.IssueFields{Resolution: &jira.Resolution{Name: options.StateAfterClose.Resolution}}}
							if _, err := jc.UpdateIssue(&updateIssue); err != nil {
								log.WithError(err).Warn("Unexpected error updating jira issue.")
								return comment(formatError(fmt.Sprintf("updating to the %s resolution", options.StateAfterClose.Resolution), jc.JiraURL(), e.key, err))
							}
						}
						response += fmt.Sprintf(" All external bug links have been closed. The bug has been moved to the %s state.", options.StateAfterClose)
					}
					jiraComment := &jira.Comment{Body: fmt.Sprintf("Bug status changed to %s as previous linked PR https://github.com/%s/%s/pull/%d has been closed", options.StateAfterClose.Status, e.org, e.repo, e.number), Visibility: PrivateVisibility}
					if _, err := jc.AddComment(bug.ID, jiraComment); err != nil {
						response += "\nWarning: Failed to comment on Jira bug with reason for changed state."
					}
				}
			}
		}
		if changed {
			return comment(response)
		}
	}
	return nil
}

func isBugAllowed(issue *jira.Issue, allowedSecurityLevel []string) (bool, error) {
	// if no allowed visibilities are listed, assume all visibilities are allowed
	if len(allowedSecurityLevel) == 0 {
		return true, nil
	}

	level, err := jiraclient.GetIssueSecurityLevel(issue)
	if err != nil {
		return false, fmt.Errorf("failed to get security level: %w", err)
	}
	if level == nil {
		// default security level is empty; make a temporary "default" security level for this check
		level = &jiraclient.SecurityLevel{Name: "default"}
	}
	found := false
	for _, allowed := range allowedSecurityLevel {
		if level.Name == allowed {
			found = true
			break
		}
	}
	return found, nil
}
