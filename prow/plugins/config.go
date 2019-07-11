/*
Copyright 2018 The Kubernetes Authors.

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

package plugins

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/labels"
)

const (
	defaultBlunderbussReviewerCount = 2
)

// Configuration is the top-level serialization target for plugin Configuration.
type Configuration struct {
	// Plugins is a map of repositories (eg "k/k") to lists of
	// plugin names.
	// You can find a comprehensive list of the default avaulable plugins here
	// https://github.com/kubernetes/test-infra/tree/master/prow/plugins
	// note that you're also able to add external plugins.
	Plugins map[string][]string `json:"plugins,omitempty"`

	// ExternalPlugins is a map of repositories (eg "k/k") to lists of
	// external plugins.
	ExternalPlugins map[string][]ExternalPlugin `json:"external_plugins,omitempty"`

	// Owners contains configuration related to handling OWNERS files.
	Owners Owners `json:"owners,omitempty"`

	// Built-in plugins specific configuration.
	Approve                    []Approve              `json:"approve,omitempty"`
	UseDeprecatedSelfApprove   bool                   `json:"use_deprecated_2018_implicit_self_approve_default_migrate_before_july_2019,omitempty"`
	UseDeprecatedReviewApprove bool                   `json:"use_deprecated_2018_review_acts_as_approve_default_migrate_before_july_2019,omitempty"`
	Blockades                  []Blockade             `json:"blockades,omitempty"`
	Blunderbuss                Blunderbuss            `json:"blunderbuss,omitempty"`
	Bugzilla                   Bugzilla               `json:"bugzilla"`
	Cat                        Cat                    `json:"cat,omitempty"`
	CherryPickUnapproved       CherryPickUnapproved   `json:"cherry_pick_unapproved,omitempty"`
	ConfigUpdater              ConfigUpdater          `json:"config_updater,omitempty"`
	Golint                     Golint                 `json:"golint"`
	Heart                      Heart                  `json:"heart,omitempty"`
	Label                      Label                  `json:"label"`
	Lgtm                       []Lgtm                 `json:"lgtm,omitempty"`
	RepoMilestone              map[string]Milestone   `json:"repo_milestone,omitempty"`
	Project                    ProjectConfig          `json:"project_config,omitempty"`
	RequireMatchingLabel       []RequireMatchingLabel `json:"require_matching_label,omitempty"`
	RequireSIG                 RequireSIG             `json:"requiresig,omitempty"`
	Slack                      Slack                  `json:"slack,omitempty"`
	SigMention                 SigMention             `json:"sigmention,omitempty"`
	Size                       Size                   `json:"size"`
	Triggers                   []Trigger              `json:"triggers,omitempty"`
	Welcome                    []Welcome              `json:"welcome,omitempty"`
}

// Golint holds configuration for the golint plugin
type Golint struct {
	// MinimumConfidence is the smallest permissible confidence
	// in (0,1] over which problems will be printed. Defaults to
	// 0.8, as does the `go lint` tool.
	MinimumConfidence *float64 `json:"minimum_confidence,omitempty"`
}

// ExternalPlugin holds configuration for registering an external
// plugin in prow.
type ExternalPlugin struct {
	// Name of the plugin.
	Name string `json:"name"`
	// Endpoint is the location of the external plugin. Defaults to
	// the name of the plugin, ie. "http://{{name}}".
	Endpoint string `json:"endpoint,omitempty"`
	// Events are the events that need to be demuxed by the hook
	// server to the external plugin. If no events are specified,
	// everything is sent.
	Events []string `json:"events,omitempty"`
}

// Blunderbuss defines configuration for the blunderbuss plugin.
type Blunderbuss struct {
	// ReviewerCount is the minimum number of reviewers to request
	// reviews from. Defaults to requesting reviews from 2 reviewers
	// if FileWeightCount is not set.
	ReviewerCount *int `json:"request_count,omitempty"`
	// MaxReviewerCount is the maximum number of reviewers to request
	// reviews from. Defaults to 0 meaning no limit.
	MaxReviewerCount int `json:"max_request_count,omitempty"`
	// FileWeightCount is the maximum number of reviewers to request
	// reviews from. Selects reviewers based on file weighting.
	// This and request_count are mutually exclusive options.
	FileWeightCount *int `json:"file_weight_count,omitempty"`
	// ExcludeApprovers controls whether approvers are considered to be
	// reviewers. By default, approvers are considered as reviewers if
	// insufficient reviewers are available. If ExcludeApprovers is true,
	// approvers will never be considered as reviewers.
	ExcludeApprovers bool `json:"exclude_approvers,omitempty"`
	// UseStatusAvailability controls whether blunderbuss will consider GitHub's
	// status availability when requesting reviews for users. This will use at one
	// additional token per successful reviewer (and potentially more depending on
	// how many busy reviewers it had to pass over).
	UseStatusAvailability bool `json:"use_status_availability,omitempty"`
}

// Owners contains configuration related to handling OWNERS files.
type Owners struct {
	// MDYAMLRepos is a list of org and org/repo strings specifying the repos that support YAML
	// OWNERS config headers at the top of markdown (*.md) files. These headers function just like
	// the config in an OWNERS file, but only apply to the file itself instead of the entire
	// directory and all sub-directories.
	// The yaml header must be at the start of the file and be bracketed with "---" like so:
	/*
		---
		approvers:
		- mikedanese
		- thockin

		---
	*/
	MDYAMLRepos []string `json:"mdyamlrepos,omitempty"`

	// SkipCollaborators disables collaborator cross-checks and forces both
	// the approve and lgtm plugins to use solely OWNERS files for access
	// control in the provided repos.
	SkipCollaborators []string `json:"skip_collaborators,omitempty"`

	// LabelsBlackList holds a list of labels that should not be present in any
	// OWNERS file, preventing their automatic addition by the owners-label plugin.
	// This check is performed by the verify-owners plugin.
	LabelsBlackList []string `json:"labels_blacklist,omitempty"`
}

// MDYAMLEnabled returns a boolean denoting if the passed repo supports YAML OWNERS config headers
// at the top of markdown (*.md) files. These function like OWNERS files but only apply to the file
// itself.
func (c *Configuration) MDYAMLEnabled(org, repo string) bool {
	full := fmt.Sprintf("%s/%s", org, repo)
	for _, elem := range c.Owners.MDYAMLRepos {
		if elem == org || elem == full {
			return true
		}
	}
	return false
}

// SkipCollaborators returns a boolean denoting if collaborator cross-checks are enabled for
// the passed repo. If it's true, approve and lgtm plugins rely solely on OWNERS files.
func (c *Configuration) SkipCollaborators(org, repo string) bool {
	full := fmt.Sprintf("%s/%s", org, repo)
	for _, elem := range c.Owners.SkipCollaborators {
		if elem == org || elem == full {
			return true
		}
	}
	return false
}

// RequireSIG specifies configuration for the require-sig plugin.
type RequireSIG struct {
	// GroupListURL is the URL where a list of the available SIGs can be found.
	GroupListURL string `json:"group_list_url,omitempty"`
}

// SigMention specifies configuration for the sigmention plugin.
type SigMention struct {
	// Regexp parses comments and should return matches to team mentions.
	// These mentions enable labeling issues or PRs with sig/team labels.
	// Furthermore, teams with the following suffixes will be mapped to
	// kind/* labels:
	//
	// * @org/team-bugs             --maps to--> kind/bug
	// * @org/team-feature-requests --maps to--> kind/feature
	// * @org/team-api-reviews      --maps to--> kind/api-change
	// * @org/team-proposals        --maps to--> kind/design
	//
	// Note that you need to make sure your regexp covers the above
	// mentions if you want to use the extra labeling. Defaults to:
	// (?m)@kubernetes/sig-([\w-]*)-(misc|test-failures|bugs|feature-requests|proposals|pr-reviews|api-reviews)
	//
	// Compiles into Re during config load.
	Regexp string         `json:"regexp,omitempty"`
	Re     *regexp.Regexp `json:"-"`
}

// Size specifies configuration for the size plugin, defining lower bounds (in # lines changed) for each size label.
// XS is assumed to be zero.
type Size struct {
	S   int `json:"s"`
	M   int `json:"m"`
	L   int `json:"l"`
	Xl  int `json:"xl"`
	Xxl int `json:"xxl"`
}

// Blockade specifies a configuration for a single blockade.
//
// The configuration for the blockade plugin is defined as a list of these structures.
type Blockade struct {
	// Repos are either of the form org/repos or just org.
	Repos []string `json:"repos,omitempty"`
	// BlockRegexps are regular expressions matching the file paths to block.
	BlockRegexps []string `json:"blockregexps,omitempty"`
	// ExceptionRegexps are regular expressions matching the file paths that are exceptions to the BlockRegexps.
	ExceptionRegexps []string `json:"exceptionregexps,omitempty"`
	// Explanation is a string that will be included in the comment left when blocking a PR. This should
	// be an explanation of why the paths specified are blockaded.
	Explanation string `json:"explanation,omitempty"`
}

// Approve specifies a configuration for a single approve.
//
// The configuration for the approve plugin is defined as a list of these structures.
type Approve struct {
	// Repos is either of the form org/repos or just org.
	Repos []string `json:"repos,omitempty"`
	// IssueRequired indicates if an associated issue is required for approval in
	// the specified repos.
	IssueRequired bool `json:"issue_required,omitempty"`

	// TODO(fejta): delete in June 2019
	DeprecatedImplicitSelfApprove *bool `json:"implicit_self_approve,omitempty"`
	// RequireSelfApproval requires PR authors to explicitly approve their PRs.
	// Otherwise the plugin assumes the author of the PR approves the changes in the PR.
	RequireSelfApproval *bool `json:"require_self_approval,omitempty"`

	// LgtmActsAsApprove indicates that the lgtm command should be used to
	// indicate approval
	LgtmActsAsApprove bool `json:"lgtm_acts_as_approve,omitempty"`

	// ReviewActsAsApprove should be replaced with its non-deprecated inverse: ignore_review_state.
	// TODO(fejta): delete in June 2019
	DeprecatedReviewActsAsApprove *bool `json:"review_acts_as_approve,omitempty"`
	// IgnoreReviewState causes the approve plugin to ignore the GitHub review state. Otherwise:
	// * an APPROVE github review is equivalent to leaving an "/approve" message.
	// * A REQUEST_CHANGES github review is equivalent to leaving an /approve cancel" message.
	IgnoreReviewState *bool `json:"ignore_review_state,omitempty"`
}

var (
	warnImplicitSelfApprove time.Time
	warnReviewActsAsApprove time.Time
)

func (a Approve) HasSelfApproval() bool {
	if a.DeprecatedImplicitSelfApprove != nil {
		warnDeprecated(&warnImplicitSelfApprove, 5*time.Minute, "Please update plugins.yaml to use require_self_approval instead of the deprecated implicit_self_approve before June 2019")
		return *a.DeprecatedImplicitSelfApprove
	} else if a.RequireSelfApproval != nil {
		return !*a.RequireSelfApproval
	}
	return true
}

func (a Approve) ConsiderReviewState() bool {
	if a.DeprecatedReviewActsAsApprove != nil {
		warnDeprecated(&warnReviewActsAsApprove, 5*time.Minute, "Please update plugins.yaml to use ignore_review_state instead of the deprecated review_acts_as_approve before June 2019")
		return *a.DeprecatedReviewActsAsApprove
	} else if a.IgnoreReviewState != nil {
		return !*a.IgnoreReviewState
	}
	return true
}

// Lgtm specifies a configuration for a single lgtm.
// The configuration for the lgtm plugin is defined as a list of these structures.
type Lgtm struct {
	// Repos is either of the form org/repos or just org.
	Repos []string `json:"repos,omitempty"`
	// ReviewActsAsLgtm indicates that a GitHub review of "approve" or "request changes"
	// acts as adding or removing the lgtm label
	ReviewActsAsLgtm bool `json:"review_acts_as_lgtm,omitempty"`
	// StoreTreeHash indicates if tree_hash should be stored inside a comment to detect
	// squashed commits before removing lgtm labels
	StoreTreeHash bool `json:"store_tree_hash,omitempty"`
	// WARNING: This disables the security mechanism that prevents a malicious member (or
	// compromised GitHub account) from merging arbitrary code. Use with caution.
	//
	// StickyLgtmTeam specifies the GitHub team whose members are trusted with sticky LGTM,
	// which eliminates the need to re-lgtm minor fixes/updates.
	StickyLgtmTeam string `json:"trusted_team_for_sticky_lgtm,omitempty"`
}

// Cat contains the configuration for the cat plugin.
type Cat struct {
	// Path to file containing an api key for thecatapi.com
	KeyPath string `json:"key_path,omitempty"`
}

// Label contains the configuration for the label plugin.
type Label struct {
	// AdditionalLabels is a set of additional labels enabled for use
	// on top of the existing "kind/*", "priority/*", and "area/*" labels.
	AdditionalLabels []string `json:"additional_labels"`
}

// Trigger specifies a configuration for a single trigger.
//
// The configuration for the trigger plugin is defined as a list of these structures.
type Trigger struct {
	// Repos is either of the form org/repos or just org.
	Repos []string `json:"repos,omitempty"`
	// TrustedOrg is the org whose members' PRs will be automatically built
	// for PRs to the above repos. The default is the PR's org.
	TrustedOrg string `json:"trusted_org,omitempty"`
	// JoinOrgURL is a link that redirects users to a location where they
	// should be able to read more about joining the organization in order
	// to become trusted members. Defaults to the GitHub link of TrustedOrg.
	JoinOrgURL string `json:"join_org_url,omitempty"`
	// OnlyOrgMembers requires PRs and/or /ok-to-test comments to come from org members.
	// By default, trigger also include repo collaborators.
	OnlyOrgMembers bool `json:"only_org_members,omitempty"`
	// IgnoreOkToTest makes trigger ignore /ok-to-test comments.
	// This is a security mitigation to only allow testing from trusted users.
	IgnoreOkToTest bool `json:"ignore_ok_to_test,omitempty"`
	// ElideSkippedContexts makes trigger not post "Skipped" contexts for jobs
	// that could run but do not run.
	ElideSkippedContexts bool `json:"elide_skipped_contexts,omitempty"`
}

// Heart contains the configuration for the heart plugin.
type Heart struct {
	// Adorees is a list of GitHub logins for members
	// for whom we will add emojis to comments
	Adorees []string `json:"adorees,omitempty"`
	// CommentRegexp is the regular expression for comments
	// made by adorees that the plugin adds emojis to.
	// If not specified, the plugin will not add emojis to
	// any comments.
	// Compiles into CommentRe during config load.
	CommentRegexp string         `json:"commentregexp,omitempty"`
	CommentRe     *regexp.Regexp `json:"-"`
}

// Milestone contains the configuration options for the milestone and
// milestonestatus plugins.
type Milestone struct {
	// ID of the github team for the milestone maintainers (used for setting status labels)
	// You can curl the following endpoint in order to determine the github ID of your team
	// responsible for maintaining the milestones:
	// curl -H "Authorization: token <token>" https://api.github.com/orgs/<org-name>/teams
	MaintainersID           int    `json:"maintainers_id,omitempty"`
	MaintainersTeam         string `json:"maintainers_team,omitempty"`
	MaintainersFriendlyName string `json:"maintainers_friendly_name,omitempty"`
}

// Slack contains the configuration for the slack plugin.
type Slack struct {
	MentionChannels []string       `json:"mentionchannels,omitempty"`
	MergeWarnings   []MergeWarning `json:"mergewarnings,omitempty"`
}

// ConfigMapSpec contains configuration options for the configMap being updated
// by the config-updater plugin.
type ConfigMapSpec struct {
	// Name of ConfigMap
	Name string `json:"name"`
	// Key is the key in the ConfigMap to update with the file contents.
	// If no explicit key is given, the basename of the file will be used.
	Key string `json:"key,omitempty"`
	// Namespace in which the configMap needs to be deployed. If no namespace is specified
	// it will be deployed to the ProwJobNamespace.
	Namespace string `json:"namespace,omitempty"`
	// Namespaces in which the configMap needs to be deployed, in addition to the above
	// namespace provided, or the default if it is not set.
	AdditionalNamespaces []string `json:"additional_namespaces,omitempty"`
	// GZIP toggles whether the key's data should be GZIP'd before being stored
	// If set to false and the global GZIP option is enabled, this file will
	// will not be GZIP'd.
	GZIP *bool `json:"gzip,omitempty"`
	// Namespaces is the fully resolved list of Namespaces to deploy the ConfigMap in
	Namespaces []string `json:"-"`
}

// ConfigUpdater contains the configuration for the config-updater plugin.
type ConfigUpdater struct {
	// A map of filename => ConfigMapSpec.
	// Whenever a commit changes filename, prow will update the corresponding configmap.
	// map[string]ConfigMapSpec{ "/my/path.yaml": {Name: "foo", Namespace: "otherNamespace" }}
	// will result in replacing the foo configmap whenever path.yaml changes
	Maps map[string]ConfigMapSpec `json:"maps,omitempty"`
	// The location of the prow configuration file inside the repository
	// where the config-updater plugin is enabled. This needs to be relative
	// to the root of the repository, eg. "prow/config.yaml" will match
	// github.com/kubernetes/test-infra/prow/config.yaml assuming the config-updater
	// plugin is enabled for kubernetes/test-infra. Defaults to "prow/config.yaml".
	ConfigFile string `json:"config_file,omitempty"`
	// The location of the prow plugin configuration file inside the repository
	// where the config-updater plugin is enabled. This needs to be relative
	// to the root of the repository, eg. "prow/plugins.yaml" will match
	// github.com/kubernetes/test-infra/prow/plugins.yaml assuming the config-updater
	// plugin is enabled for kubernetes/test-infra. Defaults to "prow/plugins.yaml".
	PluginFile string `json:"plugin_file,omitempty"`
	// If GZIP is true then files will be gzipped before insertion into
	// their corresponding configmap
	GZIP bool `json:"gzip"`
}

// ProjectConfig contains the configuration options for the project plugin
type ProjectConfig struct {
	// Org level configs for github projects; key is org name
	Orgs map[string]ProjectOrgConfig `json:"project_org_configs,omitempty"`
}

// ProjectOrgConfig holds the github project config for an entire org.
// This can be overridden by ProjectRepoConfig.
type ProjectOrgConfig struct {
	// ID of the github project maintainer team for a give project or org
	MaintainerTeamID int `json:"org_maintainers_team_id,omitempty"`
	// A map of project name to default column; an issue/PR will be added
	// to the default column if column name is not provided in the command
	ProjectColumnMap map[string]string `json:"org_default_column_map,omitempty"`
	// Repo level configs for github projects; key is repo name
	Repos map[string]ProjectRepoConfig `json:"project_repo_configs,omitempty"`
}

// ProjectRepoConfig holds the github project config for a github project.
type ProjectRepoConfig struct {
	// ID of the github project maintainer team for a give project or org
	MaintainerTeamID int `json:"repo_maintainers_team_id,omitempty"`
	// A map of project name to default column; an issue/PR will be added
	// to the default column if column name is not provided in the command
	ProjectColumnMap map[string]string `json:"repo_default_column_map,omitempty"`
}

// MergeWarning is a config for the slackevents plugin's manual merge warnings.
// If a PR is pushed to any of the repos listed in the config then send messages
// to the all the slack channels listed if pusher is NOT in the whitelist.
type MergeWarning struct {
	// Repos is either of the form org/repos or just org.
	Repos []string `json:"repos,omitempty"`
	// List of channels on which a event is published.
	Channels []string `json:"channels,omitempty"`
	// A slack event is published if the user is not part of the WhiteList.
	WhiteList []string `json:"whitelist,omitempty"`
	// A slack event is published if the user is not on the branch whitelist
	BranchWhiteList map[string][]string `json:"branch_whitelist,omitempty"`
}

// Welcome is config for the welcome plugin.
type Welcome struct {
	// Repos is either of the form org/repos or just org.
	Repos []string `json:"repos,omitempty"`
	// MessageTemplate is the welcome message template to post on new-contributor PRs
	// For the info struct see prow/plugins/welcome/welcome.go's PRInfo
	MessageTemplate string `json:"message_template,omitempty"`
}

// CherryPickUnapproved is the config for the cherrypick-unapproved plugin.
type CherryPickUnapproved struct {
	// BranchRegexp is the regular expression for branch names such that
	// the plugin treats only PRs against these branch names as cherrypick PRs.
	// Compiles into BranchRe during config load.
	BranchRegexp string         `json:"branchregexp,omitempty"`
	BranchRe     *regexp.Regexp `json:"-"`
	// Comment is the comment added by the plugin while adding the
	// `do-not-merge/cherry-pick-not-approved` label.
	Comment string `json:"comment,omitempty"`
}

// RequireMatchingLabel is the config for the require-matching-label plugin.
type RequireMatchingLabel struct {
	// Org is the GitHub organization that this config applies to.
	Org string `json:"org,omitempty"`
	// Repo is the GitHub repository within Org that this config applies to.
	// This fields may be omitted to apply this config across all repos in Org.
	Repo string `json:"repo,omitempty"`
	// Branch is the branch ref of PRs that this config applies to.
	// This field is only valid if `prs: true` and may be omitted to apply this
	// config across all branches in the repo or org.
	Branch string `json:"branch,omitempty"`
	// PRs is a bool indicating if this config applies to PRs.
	PRs bool `json:"prs,omitempty"`
	// Issues is a bool indicating if this config applies to issues.
	Issues bool `json:"issues,omitempty"`

	// Regexp is the string specifying the regular expression used to look for
	// matching labels.
	Regexp string `json:"regexp,omitempty"`
	// Re is the compiled version of Regexp. It should not be specified in config.
	Re *regexp.Regexp `json:"-"`

	// MissingLabel is the label to apply if an issue does not have any label
	// matching the Regexp.
	MissingLabel string `json:"missing_label,omitempty"`
	// MissingComment is the comment to post when we add the MissingLabel to an
	// issue. This is typically used to explain why MissingLabel was added and
	// how to move forward.
	// This field is optional. If unspecified, no comment is created when labeling.
	MissingComment string `json:"missing_comment,omitempty"`

	// GracePeriod is the amount of time to wait before processing newly opened
	// or reopened issues and PRs. This delay allows other automation to apply
	// labels before we look for matching labels.
	// Defaults to '5s'.
	GracePeriod         string        `json:"grace_period,omitempty"`
	GracePeriodDuration time.Duration `json:"-"`
}

// validate checks the following properties:
// - Org, Regexp, MissingLabel, and GracePeriod must be non-empty.
// - Repo does not contain a '/' (should use Org+Repo).
// - At least one of PRs or Issues must be true.
// - Branch only specified if 'prs: true'
// - MissingLabel must not match Regexp.
func (r RequireMatchingLabel) validate() error {
	if r.Org == "" {
		return errors.New("must specify 'org'")
	}
	if strings.Contains(r.Repo, "/") {
		return errors.New("'repo' may not contain '/'; specify the organization with 'org'")
	}
	if r.Regexp == "" {
		return errors.New("must specify 'regexp'")
	}
	if r.MissingLabel == "" {
		return errors.New("must specify 'missing_label'")
	}
	if r.GracePeriod == "" {
		return errors.New("must specify 'grace_period'")
	}
	if !r.PRs && !r.Issues {
		return errors.New("must specify 'prs: true' and/or 'issues: true'")
	}
	if !r.PRs && r.Branch != "" {
		return errors.New("branch cannot be specified without `prs: true'")
	}
	if r.Re.MatchString(r.MissingLabel) {
		return errors.New("'regexp' must not match 'missing_label'")
	}
	return nil
}

var warnLock sync.RWMutex // Rare updates and concurrent readers, so reuse the same lock

// warnDeprecated prints a deprecation warning for a particular configuration
// option.
func warnDeprecated(last *time.Time, freq time.Duration, msg string) {
	// have we warned within the last freq?
	warnLock.RLock()
	fresh := time.Now().Sub(*last) <= freq
	warnLock.RUnlock()
	if fresh { // we've warned recently
		return
	}
	// Warning is stale, will we win the race to warn?
	warnLock.Lock()
	defer warnLock.Unlock()
	now := time.Now()           // Recalculate now, we might wait awhile for the lock
	if now.Sub(*last) <= freq { // Nope, we lost
		return
	}
	*last = now
	logrus.Warn(msg)
}

// Describe generates a human readable description of the behavior that this
// configuration specifies.
func (r RequireMatchingLabel) Describe() string {
	str := &strings.Builder{}
	fmt.Fprintf(str, "Applies the '%s' label ", r.MissingLabel)
	if r.MissingComment == "" {
		fmt.Fprint(str, "to ")
	} else {
		fmt.Fprint(str, "and comments on ")
	}

	if r.Issues {
		fmt.Fprint(str, "Issues ")
		if r.PRs {
			fmt.Fprint(str, "and ")
		}
	}
	if r.PRs {
		if r.Branch != "" {
			fmt.Fprintf(str, "'%s' branch ", r.Branch)
		}
		fmt.Fprint(str, "PRs ")
	}

	if r.Repo == "" {
		fmt.Fprintf(str, "in the '%s' GitHub org ", r.Org)
	} else {
		fmt.Fprintf(str, "in the '%s/%s' GitHub repo ", r.Org, r.Repo)
	}
	fmt.Fprintf(str, "that have no labels matching the regular expression '%s'.", r.Regexp)
	return str.String()
}

// TriggerFor finds the Trigger for a repo, if one exists
// a trigger can be listed for the repo itself or for the
// owning organization
func (c *Configuration) TriggerFor(org, repo string) Trigger {
	for _, tr := range c.Triggers {
		for _, r := range tr.Repos {
			if r == org || r == fmt.Sprintf("%s/%s", org, repo) {
				return tr
			}
		}
	}
	return Trigger{}
}

// EnabledReposForPlugin returns the orgs and repos that have enabled the passed plugin.
func (c *Configuration) EnabledReposForPlugin(plugin string) (orgs, repos []string) {
	for repo, plugins := range c.Plugins {
		found := false
		for _, candidate := range plugins {
			if candidate == plugin {
				found = true
				break
			}
		}
		if found {
			if strings.Contains(repo, "/") {
				repos = append(repos, repo)
			} else {
				orgs = append(orgs, repo)
			}
		}
	}
	return
}

// EnabledReposForExternalPlugin returns the orgs and repos that have enabled the passed
// external plugin.
func (c *Configuration) EnabledReposForExternalPlugin(plugin string) (orgs, repos []string) {
	for repo, plugins := range c.ExternalPlugins {
		found := false
		for _, candidate := range plugins {
			if candidate.Name == plugin {
				found = true
				break
			}
		}
		if found {
			if strings.Contains(repo, "/") {
				repos = append(repos, repo)
			} else {
				orgs = append(orgs, repo)
			}
		}
	}
	return
}

// SetDefaults sets default options for config updating
func (c *ConfigUpdater) SetDefaults() {
	if len(c.Maps) == 0 {
		cf := c.ConfigFile
		if cf == "" {
			cf = "prow/config.yaml"
		} else {
			logrus.Warnf(`config_file is deprecated, please switch to "maps": {"%s": "config"} before July 2018`, cf)
		}
		pf := c.PluginFile
		if pf == "" {
			pf = "prow/plugins.yaml"
		} else {
			logrus.Warnf(`plugin_file is deprecated, please switch to "maps": {"%s": "plugins"} before July 2018`, pf)
		}
		c.Maps = map[string]ConfigMapSpec{
			cf: {
				Name: "config",
			},
			pf: {
				Name: "plugins",
			},
		}
	}

	for name, spec := range c.Maps {
		spec.Namespaces = append([]string{spec.Namespace}, spec.AdditionalNamespaces...)
		c.Maps[name] = spec
	}
}

func (c *Configuration) setDefaults() {
	c.ConfigUpdater.SetDefaults()

	for repo, plugins := range c.ExternalPlugins {
		for i, p := range plugins {
			if p.Endpoint != "" {
				continue
			}
			c.ExternalPlugins[repo][i].Endpoint = fmt.Sprintf("http://%s", p.Name)
		}
	}
	if c.Blunderbuss.ReviewerCount == nil && c.Blunderbuss.FileWeightCount == nil {
		c.Blunderbuss.ReviewerCount = new(int)
		*c.Blunderbuss.ReviewerCount = defaultBlunderbussReviewerCount
	}
	for i, trigger := range c.Triggers {
		if trigger.TrustedOrg == "" || trigger.JoinOrgURL != "" {
			continue
		}
		c.Triggers[i].JoinOrgURL = fmt.Sprintf("https://github.com/orgs/%s/people", trigger.TrustedOrg)
	}
	if c.SigMention.Regexp == "" {
		c.SigMention.Regexp = `(?m)@kubernetes/sig-([\w-]*)-(misc|test-failures|bugs|feature-requests|proposals|pr-reviews|api-reviews)`
	}
	if c.Owners.LabelsBlackList == nil {
		c.Owners.LabelsBlackList = []string{labels.Approved, labels.LGTM}
	}
	for _, milestone := range c.RepoMilestone {
		if milestone.MaintainersFriendlyName == "" {
			milestone.MaintainersFriendlyName = "SIG Chairs/TLs"
		}
	}
	if c.CherryPickUnapproved.BranchRegexp == "" {
		c.CherryPickUnapproved.BranchRegexp = `^release-.*$`
	}
	if c.CherryPickUnapproved.Comment == "" {
		c.CherryPickUnapproved.Comment = `This PR is not for the master branch but does not have the ` + "`cherry-pick-approved`" + `  label. Adding the ` + "`do-not-merge/cherry-pick-not-approved`" + `  label.

To approve the cherry-pick, please ping the *kubernetes/patch-release-team* in a comment when ready.

See also [Kubernetes Patch Releases](https://github.com/kubernetes/sig-release/blob/master/releases/patch-releases.md)`
	}

	for i, rml := range c.RequireMatchingLabel {
		if rml.GracePeriod == "" {
			c.RequireMatchingLabel[i].GracePeriod = "5s"
		}
	}
}

// validatePlugins will return error if
// there are unknown or duplicated plugins.
func validatePlugins(plugins map[string][]string) error {
	var errors []string
	for _, configuration := range plugins {
		for _, plugin := range configuration {
			if _, ok := pluginHelp[plugin]; !ok {
				errors = append(errors, fmt.Sprintf("unknown plugin: %s", plugin))
			}
		}
	}
	for repo, repoConfig := range plugins {
		if strings.Contains(repo, "/") {
			org := strings.Split(repo, "/")[0]
			if dupes := findDuplicatedPluginConfig(repoConfig, plugins[org]); len(dupes) > 0 {
				errors = append(errors, fmt.Sprintf("plugins %v are duplicated for %s and %s", dupes, repo, org))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("invalid plugin configuration:\n\t%v", strings.Join(errors, "\n\t"))
	}
	return nil
}

func validateSizes(size Size) error {
	if size.S > size.M || size.M > size.L || size.L > size.Xl || size.Xl > size.Xxl {
		return errors.New("invalid size plugin configuration - one of the smaller sizes is bigger than a larger one")
	}

	return nil
}

func findDuplicatedPluginConfig(repoConfig, orgConfig []string) []string {
	var dupes []string
	for _, repoPlugin := range repoConfig {
		for _, orgPlugin := range orgConfig {
			if repoPlugin == orgPlugin {
				dupes = append(dupes, repoPlugin)
			}
		}
	}

	return dupes
}

func validateExternalPlugins(pluginMap map[string][]ExternalPlugin) error {
	var errors []string

	for repo, plugins := range pluginMap {
		if !strings.Contains(repo, "/") {
			continue
		}
		org := strings.Split(repo, "/")[0]

		var orgConfig []string
		for _, p := range pluginMap[org] {
			orgConfig = append(orgConfig, p.Name)
		}

		var repoConfig []string
		for _, p := range plugins {
			repoConfig = append(repoConfig, p.Name)
		}

		if dupes := findDuplicatedPluginConfig(repoConfig, orgConfig); len(dupes) > 0 {
			errors = append(errors, fmt.Sprintf("external plugins %v are duplicated for %s and %s", dupes, repo, org))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("invalid plugin configuration:\n\t%v", strings.Join(errors, "\n\t"))
	}
	return nil
}

var warnBlunderbussFileWeightCount time.Time

func validateBlunderbuss(b *Blunderbuss) error {
	if b.ReviewerCount != nil && b.FileWeightCount != nil {
		return errors.New("cannot use both request_count and file_weight_count in blunderbuss")
	}
	if b.ReviewerCount != nil && *b.ReviewerCount < 1 {
		return fmt.Errorf("invalid request_count: %v (needs to be positive)", *b.ReviewerCount)
	}
	if b.FileWeightCount != nil && *b.FileWeightCount < 1 {
		return fmt.Errorf("invalid file_weight_count: %v (needs to be positive)", *b.FileWeightCount)
	}
	if b.FileWeightCount != nil {
		warnDeprecated(&warnBlunderbussFileWeightCount, 5*time.Minute, "file_weight_count is being deprecated in favour of max_request_count. Please ensure your configuration is updated before the end of May 2019.")
	}
	return nil
}

func validateConfigUpdater(updater *ConfigUpdater) error {
	files := sets.NewString()
	configMapKeys := map[string]sets.String{}
	for file, config := range updater.Maps {
		if files.Has(file) {
			return fmt.Errorf("file %s listed more than once in config updater config", file)
		}
		files.Insert(file)

		key := config.Key
		if key == "" {
			key = path.Base(file)
		}

		if _, ok := configMapKeys[config.Name]; ok {
			if configMapKeys[config.Name].Has(key) {
				return fmt.Errorf("key %s in configmap %s updated with more than one file", key, config.Name)
			}
			configMapKeys[config.Name].Insert(key)
		} else {
			configMapKeys[config.Name] = sets.NewString(key)
		}
	}
	return nil
}

func validateRequireMatchingLabel(rs []RequireMatchingLabel) error {
	for i, r := range rs {
		if err := r.validate(); err != nil {
			return fmt.Errorf("error validating require_matching_label config #%d: %v", i, err)
		}
	}
	return nil
}

func compileRegexpsAndDurations(pc *Configuration) error {
	cRe, err := regexp.Compile(pc.SigMention.Regexp)
	if err != nil {
		return err
	}
	pc.SigMention.Re = cRe

	branchRe, err := regexp.Compile(pc.CherryPickUnapproved.BranchRegexp)
	if err != nil {
		return err
	}
	pc.CherryPickUnapproved.BranchRe = branchRe

	commentRe, err := regexp.Compile(pc.Heart.CommentRegexp)
	if err != nil {
		return err
	}
	pc.Heart.CommentRe = commentRe

	rs := pc.RequireMatchingLabel
	for i := range rs {
		re, err := regexp.Compile(rs[i].Regexp)
		if err != nil {
			return fmt.Errorf("failed to compile label regexp: %q, error: %v", rs[i].Regexp, err)
		}
		rs[i].Re = re

		var dur time.Duration
		dur, err = time.ParseDuration(rs[i].GracePeriod)
		if err != nil {
			return fmt.Errorf("failed to compile grace period duration: %q, error: %v", rs[i].GracePeriod, err)
		}
		rs[i].GracePeriodDuration = dur
	}
	return nil
}

func (c *Configuration) Validate() error {
	if len(c.Plugins) == 0 {
		logrus.Warn("no plugins specified-- check syntax?")
	}

	// Defaulting should run before validation.
	c.setDefaults()
	// Regexp compilation should run after defaulting, but before validation.
	if err := compileRegexpsAndDurations(c); err != nil {
		return err
	}

	if err := validatePlugins(c.Plugins); err != nil {
		return err
	}
	if err := validateExternalPlugins(c.ExternalPlugins); err != nil {
		return err
	}
	if err := validateBlunderbuss(&c.Blunderbuss); err != nil {
		return err
	}
	if err := validateConfigUpdater(&c.ConfigUpdater); err != nil {
		return err
	}
	if err := validateSizes(c.Size); err != nil {
		return err
	}
	if err := validateRequireMatchingLabel(c.RequireMatchingLabel); err != nil {
		return err
	}

	return nil
}

func (pluginConfig *ProjectConfig) GetMaintainerTeam(org string, repo string) int {
	for orgName, orgConfig := range pluginConfig.Orgs {
		if org == orgName {
			// look for repo level configs first because repo level config overrides org level configs
			for repoName, repoConfig := range orgConfig.Repos {
				if repo == repoName {
					return repoConfig.MaintainerTeamID
				}
			}
			return orgConfig.MaintainerTeamID
		}
	}
	return -1
}

func (pluginConfig *ProjectConfig) GetColumnMap(org string, repo string) map[string]string {
	for orgName, orgConfig := range pluginConfig.Orgs {
		if org == orgName {
			for repoName, repoConfig := range orgConfig.Repos {
				if repo == repoName {
					return repoConfig.ProjectColumnMap
				}
			}
			return orgConfig.ProjectColumnMap
		}
	}
	return nil
}

func (pluginConfig *ProjectConfig) GetOrgColumnMap(org string) map[string]string {
	for orgName, orgConfig := range pluginConfig.Orgs {
		if org == orgName {
			return orgConfig.ProjectColumnMap
		}
	}
	return nil
}

// Bugzilla holds options for checking Bugzilla bugs in a defaulting hierarchy.
type Bugzilla struct {
	// Default settings mapped by branch in any repo in any org.
	// The `*` wildcard will apply to all branches.
	Default map[string]BugzillaBranchOptions `json:"default"`
	// Options for specific orgs. The `*` wildcard will apply to all orgs.
	Orgs map[string]BugzillaOrgOptions `json:"orgs"`
}

// BugzillaOrgOptions holds options for checking Bugzilla bugs for an org.
type BugzillaOrgOptions struct {
	// Default settings mapped by branch in any repo in this org.
	// The `*` wildcard will apply to all branches.
	Default map[string]BugzillaBranchOptions `json:"default"`
	// Options for specific repos. The `*` wildcard will apply to all repos.
	Repos map[string]BugzillaRepoOptions `json:"repos"`
}

// BugzillaRepoOptions holds options for checking Bugzilla bugs for a repo.
type BugzillaRepoOptions struct {
	// Options for specific branches in this repo.
	// The `*` wildcard will apply to all branches.
	Branches map[string]BugzillaBranchOptions `json:"branches"`
}

// BugzillaBranchOptions describes how to check if a Bugzilla bug is valid or not.
type BugzillaBranchOptions struct {
	// ValidateByDefault determines whether a validation check is run for all pull
	// requests by default
	ValidateByDefault *bool `json:"validate_by_default,omitempty"`

	// IsOpen determines whether a bug needs to be open to be valid
	IsOpen *bool `json:"is_open,omitempty"`
	// TargetRelease determines which release a bug needs to target to be valid
	TargetRelease *string `json:"target_release,omitempty"`
	// Statuses determine which statuses a bug may have to be valid
	Statuses *[]string `json:"statuses,omitempty"`
	// DependentBugStatuses determine which statuses a bug's dependent bugs may have
	// to deem the child bug valid
	DependentBugStatuses *[]string `json:"dependent_bug_statuses,omitempty"`

	// StatusAfterValidation is the status which the bug will be moved to after being
	// deemed valid and linked to a PR. Will implicitly be considered a part of `statuses`
	// if others are set.
	StatusAfterValidation *string `json:"status_after_validation,omitempty"`
	// AddExternalLink determines whether the pull request will be added to the Bugzilla
	// bug using the ExternalBug tracker API after being validated
	AddExternalLink *bool `json:"add_external_link,omitempty"`
	// StatusAfterMerge is the status which the bug will be moved to after all pull requests
	// in the external bug tracker have been merged.
	StatusAfterMerge *string `json:"status_after_merge,omitempty"`
}

func (o BugzillaBranchOptions) matches(other BugzillaBranchOptions) bool {
	validateByDefaultMatch := o.ValidateByDefault == nil && other.ValidateByDefault == nil ||
		(o.ValidateByDefault != nil && other.ValidateByDefault != nil && *o.ValidateByDefault == *other.ValidateByDefault)
	isOpenMatch := o.IsOpen == nil && other.IsOpen == nil ||
		(o.IsOpen != nil && other.IsOpen != nil && *o.IsOpen == *other.IsOpen)
	targetReleaseMatch := o.TargetRelease == nil && other.TargetRelease == nil ||
		(o.TargetRelease != nil && other.TargetRelease != nil && *o.TargetRelease == *other.TargetRelease)
	statusesMatch := o.Statuses == nil && other.Statuses == nil ||
		(o.Statuses != nil && other.Statuses != nil && sets.NewString(*o.Statuses...).Equal(sets.NewString(*other.Statuses...)))
	dependentBugStatusesMatch := o.DependentBugStatuses == nil && other.DependentBugStatuses == nil ||
		(o.DependentBugStatuses != nil && other.DependentBugStatuses != nil && sets.NewString(*o.DependentBugStatuses...).Equal(sets.NewString(*other.DependentBugStatuses...)))
	statusesAfterValidationMatch := o.StatusAfterValidation == nil && other.StatusAfterValidation == nil ||
		(o.StatusAfterValidation != nil && other.StatusAfterValidation != nil && *o.StatusAfterValidation == *other.StatusAfterValidation)
	addExternalLinkMatch := o.AddExternalLink == nil && other.AddExternalLink == nil ||
		(o.AddExternalLink != nil && other.AddExternalLink != nil && *o.AddExternalLink == *other.AddExternalLink)
	statusAfterMergeMatch := o.StatusAfterMerge == nil && other.StatusAfterMerge == nil ||
		(o.StatusAfterMerge != nil && other.StatusAfterMerge != nil && *o.StatusAfterMerge == *other.StatusAfterMerge)
	return validateByDefaultMatch && isOpenMatch && targetReleaseMatch && statusesMatch && dependentBugStatusesMatch && statusesAfterValidationMatch && addExternalLinkMatch && statusAfterMergeMatch
}

const BugzillaOptionsWildcard = `*`

// OptionsForItem resolves a set of options for an item, honoring
// the `*` wildcard and doing defaulting if it is present with the
// item itself.
func OptionsForItem(item string, config map[string]BugzillaBranchOptions) BugzillaBranchOptions {
	return ResolveBugzillaOptions(config[BugzillaOptionsWildcard], config[item])
}

// ResolveBugzillaOptions implements defaulting for a parent/child configuration,
// preferring child fields where set.
func ResolveBugzillaOptions(parent, child BugzillaBranchOptions) BugzillaBranchOptions {
	output := BugzillaBranchOptions{}

	// populate with the parent
	if parent.ValidateByDefault != nil {
		output.ValidateByDefault = parent.ValidateByDefault
	}
	if parent.IsOpen != nil {
		output.IsOpen = parent.IsOpen
	}
	if parent.TargetRelease != nil {
		output.TargetRelease = parent.TargetRelease
	}
	if parent.Statuses != nil {
		output.Statuses = parent.Statuses
	}
	if parent.DependentBugStatuses != nil {
		output.DependentBugStatuses = parent.DependentBugStatuses
	}
	if parent.StatusAfterValidation != nil {
		output.StatusAfterValidation = parent.StatusAfterValidation
	}
	if parent.AddExternalLink != nil {
		output.AddExternalLink = parent.AddExternalLink
	}
	if parent.StatusAfterMerge != nil {
		output.StatusAfterMerge = parent.StatusAfterMerge
	}

	//override with the child
	if child.ValidateByDefault != nil {
		output.ValidateByDefault = child.ValidateByDefault
	}
	if child.IsOpen != nil {
		output.IsOpen = child.IsOpen
	}
	if child.TargetRelease != nil {
		output.TargetRelease = child.TargetRelease
	}
	if child.Statuses != nil {
		output.Statuses = child.Statuses
	}
	if child.DependentBugStatuses != nil {
		output.DependentBugStatuses = child.DependentBugStatuses
	}
	if child.StatusAfterValidation != nil {
		output.StatusAfterValidation = child.StatusAfterValidation
	}
	if child.AddExternalLink != nil {
		output.AddExternalLink = child.AddExternalLink
	}
	if child.StatusAfterMerge != nil {
		output.StatusAfterMerge = child.StatusAfterMerge
	}

	return output
}

// OptionsForBranch determines the criteria for a valid Bugzilla bug on a branch of a repo
// by defaulting in a cascading way, in the following order (later entries override earlier
// ones), always searching for the wildcard as well as the branch name: global, then org,
// repo, and finally branch-specific configuration.
func (b *Bugzilla) OptionsForBranch(org, repo, branch string) BugzillaBranchOptions {
	options := OptionsForItem(branch, b.Default)
	orgOptions, exists := b.Orgs[org]
	if !exists {
		return options
	}
	options = ResolveBugzillaOptions(options, OptionsForItem(branch, orgOptions.Default))

	repoOptions, exists := orgOptions.Repos[repo]
	if !exists {
		return options
	}
	options = ResolveBugzillaOptions(options, OptionsForItem(branch, repoOptions.Branches))

	return options
}

// OptionsForRepo determines the criteria for a valid Bugzilla bug on branches of a repo
// by defaulting in a cascading way, in the following order (later entries override earlier
// ones), always searching for the wildcard as well as the branch name: global, then org,
// repo, and finally branch-specific configuration.
func (b *Bugzilla) OptionsForRepo(org, repo string) map[string]BugzillaBranchOptions {
	options := map[string]BugzillaBranchOptions{}
	for branch := range b.Default {
		options[branch] = b.OptionsForBranch(org, repo, branch)
	}

	orgOptions, exists := b.Orgs[org]
	if exists {
		for branch := range orgOptions.Default {
			options[branch] = b.OptionsForBranch(org, repo, branch)
		}
	}

	repoOptions, exists := orgOptions.Repos[repo]
	if exists {
		for branch := range repoOptions.Branches {
			options[branch] = b.OptionsForBranch(org, repo, branch)
		}
	}

	// if there are nested defaults there is no reason to call out branches
	// from higher levels of config
	var toDelete []string
	for branch, branchOptions := range options {
		if branchOptions.matches(options[BugzillaOptionsWildcard]) && branch != BugzillaOptionsWildcard {
			toDelete = append(toDelete, branch)
		}
	}
	for _, branch := range toDelete {
		delete(options, branch)
	}

	return options
}
