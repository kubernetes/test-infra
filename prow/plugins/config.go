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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/test-infra/prow/bugzilla"
	"k8s.io/test-infra/prow/kube"
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

	Approve                    []Approve                    `json:"approve,omitempty"`
	UseDeprecatedSelfApprove   bool                         `json:"use_deprecated_2018_implicit_self_approve_default_migrate_before_july_2019,omitempty"`
	UseDeprecatedReviewApprove bool                         `json:"use_deprecated_2018_review_acts_as_approve_default_migrate_before_july_2019,omitempty"`
	Blockades                  []Blockade                   `json:"blockades,omitempty"`
	Blunderbuss                Blunderbuss                  `json:"blunderbuss,omitempty"`
	Bugzilla                   Bugzilla                     `json:"bugzilla,omitempty"`
	Cat                        Cat                          `json:"cat,omitempty"`
	CherryPickUnapproved       CherryPickUnapproved         `json:"cherry_pick_unapproved,omitempty"`
	ConfigUpdater              ConfigUpdater                `json:"config_updater,omitempty"`
	Dco                        map[string]*Dco              `json:"dco,omitempty"`
	Golint                     Golint                       `json:"golint,omitempty"`
	Goose                      Goose                        `json:"goose,omitempty"`
	Heart                      Heart                        `json:"heart,omitempty"`
	Label                      Label                        `json:"label,omitempty"`
	Lgtm                       []Lgtm                       `json:"lgtm,omitempty"`
	MilestoneApplier           map[string]BranchToMilestone `json:"milestone_applier,omitempty"`
	RepoMilestone              map[string]Milestone         `json:"repo_milestone,omitempty"`
	Project                    ProjectConfig                `json:"project_config,omitempty"`
	ProjectManager             ProjectManager               `json:"project_manager,omitempty"`
	RequireMatchingLabel       []RequireMatchingLabel       `json:"require_matching_label,omitempty"`
	RequireSIG                 RequireSIG                   `json:"requiresig,omitempty"`
	Retitle                    Retitle                      `json:"retitle,omitempty"`
	Slack                      Slack                        `json:"slack,omitempty"`
	SigMention                 SigMention                   `json:"sigmention,omitempty"`
	Size                       Size                         `json:"size,omitempty"`
	Triggers                   []Trigger                    `json:"triggers,omitempty"`
	Welcome                    []Welcome                    `json:"welcome,omitempty"`
	Override                   Override                     `json:"override"`
	JiraLinker                 JiraLinker                   `json:"jira_linker,omitempty"`
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

// Retitle specifies configuration for the retitle plugin.
type Retitle struct {
	// AllowClosedIssues allows retitling closed/merged issues and PRs.
	AllowClosedIssues bool `json:"allow_closed_issues,omitempty"`
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
	warnImplicitSelfApprove       time.Time
	warnReviewActsAsApprove       time.Time
	warnDependentBugTargetRelease time.Time
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

// Goose contains the configuration for the goose plugin.
type Goose struct {
	// Path to file containing an api key for unsplash.com
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
	// TrustedOrg is the org whose members' PRs will be automatically built for
	// PRs to the above repos. The default is the PR's org.
	//
	// Deprecated: TrustedOrg functionality is deprecated and will be removed in
	// January 2020.
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
	// that could run but do not run. Defaults to true.
	// THIS FIELD IS DEPRECATED AND WILL BE REMOVED AFTER OCTOBER 2019.
	ElideSkippedContexts *bool `json:"elide_skipped_contexts,omitempty"`
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

// BranchToMilestone is a map of the branch name to the configured milestone for that branch.
// This is used by the milestoneapplier plugin.
type BranchToMilestone map[string]string

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
	// Clusters is a map from cluster to namespaces
	// which specifies the targets the configMap needs to be deployed, i.e., each namespace in map[cluster]
	Clusters map[string][]string `json:"clusters"`
}

// ConfigUpdater contains the configuration for the config-updater plugin.
type ConfigUpdater struct {
	// A map of filename => ConfigMapSpec.
	// Whenever a commit changes filename, prow will update the corresponding configmap.
	// map[string]ConfigMapSpec{ "/my/path.yaml": {Name: "foo", Namespace: "otherNamespace" }}
	// will result in replacing the foo configmap whenever path.yaml changes
	Maps map[string]ConfigMapSpec `json:"maps,omitempty"`
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

// ProjectManager represents the config for the ProjectManager plugin, holding top
// level config options, configuration is a hierarchial structure with top level element
// being org or org/repo with the list of projects as its children
type ProjectManager struct {
	OrgRepos map[string]ManagedOrgRepo `json:"orgsRepos,omitempty"`
}

// ManagedOrgRepo is used by the ProjectManager plugin to represent an Organisation
// or Repository with a list of Projects
type ManagedOrgRepo struct {
	Projects map[string]ManagedProject `json:"projects,omitempty"`
}

// ManagedProject is used by the ProjectManager plugin to represent a Project
// with a list of Columns
type ManagedProject struct {
	Columns []ManagedColumn `json:"columns,omitempty"`
}

// ManagedColumn is used by the ProjectQueries plugin to represent a project column
// and the conditions to add a PR to that column
type ManagedColumn struct {
	// Either of ID or Name should be specified
	ID   *int   `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	// State must be open, closed or all
	State string `json:"state,omitempty"`
	// all the labels here should match to the incoming event to be bale to add the card to the project
	Labels []string `json:"labels,omitempty"`
	// Configuration is effective is the issue events repo/Owner/Login matched the org
	Org string `json:"org,omitempty"`
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

// JiraLinker is the config for the jira-linker plugin
type JiraLinker struct {
	JiraBaseUrl string `json:"jira_base_url"`
	JiraOverrides   []JiraOverrides
}

type JiraOverrides struct {
	JiraUrl string `json:"jira_url,omitempty"`
	Repo       string `json:"repo,omitempty"`
}

// Dco is config for the DCO (https://developercertificate.org/) checker plugin.
type Dco struct {
	// SkipDCOCheckForMembers is used to skip DCO check for trusted org members
	SkipDCOCheckForMembers bool `json:"skip_dco_check_for_members,omitempty"`
	// TrustedOrg is the org whose members' commits will not be checked for DCO signoff
	// if the skip DCO option is enabled. The default is the PR's org.
	TrustedOrg string `json:"trusted_org,omitempty"`
	// SkipDCOCheckForCollaborators is used to skip DCO check for trusted org members
	SkipDCOCheckForCollaborators bool `json:"skip_dco_check_for_collaborators,omitempty"`
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

// ApproveFor finds the Approve for a repo, if one exists.
// Approval configuration can be listed for a repository
// or an organization.
func (c *Configuration) ApproveFor(org, repo string) *Approve {
	fullName := fmt.Sprintf("%s/%s", org, repo)

	a := func() *Approve {
		// First search for repo config
		for _, approve := range c.Approve {
			if !sets.NewString(approve.Repos...).Has(fullName) {
				continue
			}
			return &approve
		}

		// If you don't find anything, loop again looking for an org config
		for _, approve := range c.Approve {
			if !sets.NewString(approve.Repos...).Has(org) {
				continue
			}
			return &approve
		}

		// Return an empty config, and use plugin defaults
		return &Approve{}
	}()
	if a.DeprecatedImplicitSelfApprove == nil && a.RequireSelfApproval == nil && c.UseDeprecatedSelfApprove {
		no := false
		a.DeprecatedImplicitSelfApprove = &no
	}
	if a.DeprecatedReviewActsAsApprove == nil && a.IgnoreReviewState == nil && c.UseDeprecatedReviewApprove {
		no := false
		a.DeprecatedReviewActsAsApprove = &no
	}
	return a
}

// LgtmFor finds the Lgtm for a repo, if one exists
// a trigger can be listed for the repo itself or for the
// owning organization
func (c *Configuration) LgtmFor(org, repo string) *Lgtm {
	fullName := fmt.Sprintf("%s/%s", org, repo)
	for _, lgtm := range c.Lgtm {
		if !sets.NewString(lgtm.Repos...).Has(fullName) {
			continue
		}
		return &lgtm
	}
	// If you don't find anything, loop again looking for an org config
	for _, lgtm := range c.Lgtm {
		if !sets.NewString(lgtm.Repos...).Has(org) {
			continue
		}
		return &lgtm
	}
	return &Lgtm{}
}

// TriggerFor finds the Trigger for a repo, if one exists
// a trigger can be listed for the repo itself or for the
// owning organization
func (c *Configuration) TriggerFor(org, repo string) Trigger {
	orgRepo := fmt.Sprintf("%s/%s", org, repo)
	for _, tr := range c.Triggers {
		for _, r := range tr.Repos {
			if r == org || r == orgRepo {
				return tr
			}
		}
	}
	var tr Trigger
	tr.SetDefaults()
	return tr
}

var warnElideSkippedContexts time.Time

func (t *Trigger) SetDefaults() {
	truth := true
	if t.ElideSkippedContexts == nil {
		t.ElideSkippedContexts = &truth
	} else {
		warnDeprecated(&warnElideSkippedContexts, 5*time.Minute, "elide_skipped_contexts is deprecated and will be removed after Oct. 2019. Skipped contexts are now elided by default.")
	}

	if t.TrustedOrg != "" && t.JoinOrgURL == "" {
		t.JoinOrgURL = fmt.Sprintf("https://github.com/orgs/%s/people", t.TrustedOrg)
	}
}

// DcoFor finds the Dco for a repo, if one exists
// a Dco can be listed for the repo itself or for the
// owning organization
func (c *Configuration) DcoFor(org, repo string) *Dco {
	if c.Dco[fmt.Sprintf("%s/%s", org, repo)] != nil {
		return c.Dco[fmt.Sprintf("%s/%s", org, repo)]
	}
	if c.Dco[org] != nil {
		return c.Dco[org]
	}
	if c.Dco["*"] != nil {
		return c.Dco["*"]
	}
	return &Dco{}
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
		c.Maps = map[string]ConfigMapSpec{
			"config/prow/config.yaml": {
				Name: "config",
			},
			"config/prow/plugins.yaml": {
				Name: "plugins",
			},
		}
	}

	for name, spec := range c.Maps {
		if spec.Namespace != "" || len(spec.AdditionalNamespaces) > 0 {
			logrus.Warnf("'namespace' and 'additional_namespaces' are deprecated for config-updater plugin, use 'clusters' instead")
		}
		// as a result, namespaces will never be an empty slice (namespace in the slice could be empty string)
		// and clusters will never be an empty map (map[cluster] could be am empty slice)
		spec.Namespaces = append([]string{spec.Namespace}, spec.AdditionalNamespaces...)
		if len(spec.Clusters) == 0 {
			spec.Clusters = map[string][]string{kube.DefaultClusterAlias: spec.Namespaces}
		}
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
	for i := range c.Triggers {
		c.Triggers[i].SetDefaults()
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
		c.CherryPickUnapproved.Comment = `This PR is not for the master branch but does not have the ` + "`cherry-pick-approved`" + `  label. Adding the ` + "`do-not-merge/cherry-pick-not-approved`" + `  label.`
	}

	for i, rml := range c.RequireMatchingLabel {
		if rml.GracePeriod == "" {
			c.RequireMatchingLabel[i].GracePeriod = "5s"
		}
	}
}

// validatePluginsDupes will return an error if there are duplicated plugins.
// It is sometimes a sign of misconfiguration and is always useless for a
// plugin to be specified at both the org and repo levels.
func validatePluginsDupes(plugins map[string][]string) error {
	var errors []error
	for repo, repoConfig := range plugins {
		if strings.Contains(repo, "/") {
			org := strings.Split(repo, "/")[0]
			if dupes := findDuplicatedPluginConfig(repoConfig, plugins[org]); len(dupes) > 0 {
				errors = append(errors, fmt.Errorf("plugins %v are duplicated for %s and %s", dupes, repo, org))
			}
		}
	}
	return utilerrors.NewAggregate(errors)
}

// ValidatePluginsUnknown will return an error if there are any unrecognized
// plugins configured.
func (c *Configuration) ValidatePluginsUnknown() error {
	var errors []error
	for _, configuration := range c.Plugins {
		for _, plugin := range configuration {
			if _, ok := pluginHelp[plugin]; !ok {
				errors = append(errors, fmt.Errorf("unknown plugin: %s", plugin))
			}
		}
	}
	return utilerrors.NewAggregate(errors)
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

// ConfigMapID is a name/namespace/cluster combination that identifies a config map
type ConfigMapID struct {
	Name, Namespace, Cluster string
}

func validateConfigUpdater(updater *ConfigUpdater) error {
	updater.SetDefaults()
	configMapKeys := map[ConfigMapID]sets.String{}
	for file, config := range updater.Maps {
		for cluster, namespaces := range config.Clusters {
			for _, namespace := range namespaces {
				cmID := ConfigMapID{
					Name:      config.Name,
					Namespace: namespace,
					Cluster:   cluster,
				}

				key := config.Key
				if key == "" {
					key = path.Base(file)
				}

				if _, ok := configMapKeys[cmID]; ok {
					if configMapKeys[cmID].Has(key) {
						return fmt.Errorf("key %s in configmap %s updated with more than one file", key, config.Name)
					}
					configMapKeys[cmID].Insert(key)
				} else {
					configMapKeys[cmID] = sets.NewString(key)
				}
			}
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

func validateProjectManager(pm ProjectManager) error {

	projectConfig := pm
	// No ProjectManager configuration provided, we have nothing to validate
	if len(projectConfig.OrgRepos) == 0 {
		return nil
	}

	for orgRepoName, managedOrgRepo := range pm.OrgRepos {
		if len(managedOrgRepo.Projects) == 0 {
			return fmt.Errorf("Org/repo: %s, has no projects configured", orgRepoName)
		}
		for projectName, managedProject := range managedOrgRepo.Projects {
			var labelSets []sets.String
			if len(managedProject.Columns) == 0 {
				return fmt.Errorf("Org/repo: %s, project %s, has no columns configured", orgRepoName, projectName)
			}
			for _, managedColumn := range managedProject.Columns {
				if managedColumn.ID == nil && (len(managedColumn.Name) == 0) {
					return fmt.Errorf("Org/repo: %s, project %s, column %v, has no name/id configured", orgRepoName, projectName, managedColumn)
				}
				if len(managedColumn.Labels) == 0 {
					return fmt.Errorf("Org/repo: %s, project %s, column %s, has no labels configured", orgRepoName, projectName, managedColumn.Name)
				}
				if len(managedColumn.Org) == 0 {
					return fmt.Errorf("Org/repo: %s, project %s, column %s, has no org configured", orgRepoName, projectName, managedColumn.Name)
				}
				sSet := sets.NewString(managedColumn.Labels...)
				for _, labels := range labelSets {
					if sSet.Equal(labels) {
						return fmt.Errorf("Org/repo: %s, project %s, column %s has same labels configured as another column", orgRepoName, projectName, managedColumn.Name)
					}
				}
				labelSets = append(labelSets, sSet)
			}
		}
	}
	return nil
}

var warnTriggerTrustedOrg time.Time

func validateTrigger(triggers []Trigger) error {
	for _, trigger := range triggers {
		if trigger.TrustedOrg != "" {
			warnDeprecated(&warnTriggerTrustedOrg, 5*time.Minute, "trusted_org functionality is deprecated. Please ensure your configuration is updated before the end of December 2019.")
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

	if err := validatePluginsDupes(c.Plugins); err != nil {
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
	if err := validateProjectManager(c.ProjectManager); err != nil {
		return err
	}
	if err := validateTrigger(c.Triggers); err != nil {
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
	Default map[string]BugzillaBranchOptions `json:"default,omitempty"`
	// Options for specific orgs. The `*` wildcard will apply to all orgs.
	Orgs map[string]BugzillaOrgOptions `json:"orgs,omitempty"`
}

// BugzillaOrgOptions holds options for checking Bugzilla bugs for an org.
type BugzillaOrgOptions struct {
	// Default settings mapped by branch in any repo in this org.
	// The `*` wildcard will apply to all branches.
	Default map[string]BugzillaBranchOptions `json:"default,omitempty"`
	// Options for specific repos. The `*` wildcard will apply to all repos.
	Repos map[string]BugzillaRepoOptions `json:"repos,omitempty"`
}

// BugzillaRepoOptions holds options for checking Bugzilla bugs for a repo.
type BugzillaRepoOptions struct {
	// Options for specific branches in this repo.
	// The `*` wildcard will apply to all branches.
	Branches map[string]BugzillaBranchOptions `json:"branches,omitempty"`
}

// BugzillaBugState describes bug states in the Bugzilla plugin config, used
// for example to specify states that bugs are supposed to be in or to which
// they should be made after some action.
type BugzillaBugState struct {
	Status     string `json:"status,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

// String converts a Bugzilla state into human-readable description
func (s *BugzillaBugState) String() string {
	return bugzilla.PrettyStatus(s.Status, s.Resolution)
}

// AsBugUpdate returns a BugUpdate struct for updating a given to bug to the
// desired state. The returned struct will have only those fields set where the
// state differs from the parameter bug. If the bug state matches the desired
// state, returns nil. If the parameter bug is empty or a nil pointer, the
// returned BugUpdate will have all fields set that are set in the state.
func (s *BugzillaBugState) AsBugUpdate(bug *bugzilla.Bug) *bugzilla.BugUpdate {
	if s == nil {
		return nil
	}

	var ret *bugzilla.BugUpdate
	var update bugzilla.BugUpdate

	if s.Status != "" && (bug == nil || s.Status != bug.Status) {
		ret = &update
		update.Status = s.Status
	}
	if s.Resolution != "" && (bug == nil || s.Resolution != bug.Resolution) {
		ret = &update
		update.Resolution = s.Resolution
	}

	return ret
}

// Matches returns whether a given bug matches the state
func (s *BugzillaBugState) Matches(bug *bugzilla.Bug) bool {
	if s == nil || bug == nil {
		return false
	}
	if s.Status != "" && s.Status != bug.Status {
		return false
	}

	if s.Resolution != "" && s.Resolution != bug.Resolution {
		return false
	}
	return true
}

// BugzillaBranchOptions describes how to check if a Bugzilla bug is valid or not.
//
// Note on `Status` vs `State` fields: `State` fields implement a superset of
// functionality provided by the `Status` fields and are meant to eventually
// supersede `Status` fields. Implementations using these structures should
// *only* use `Status` fields or only `States` fields, never both. The
// implementation mirrors `Status` fields into the matching `State` fields in
// the `ResolveBugzillaOptions` method to handle existing config, and is also
// able to sufficiently resolve the presence of both types of fields.
type BugzillaBranchOptions struct {
	// ExcludeDefaults excludes defaults from more generic Bugzilla configurations.
	ExcludeDefaults *bool `json:"exclude_defaults,omitempty"`

	// ValidateByDefault determines whether a validation check is run for all pull
	// requests by default
	ValidateByDefault *bool `json:"validate_by_default,omitempty"`

	// IsOpen determines whether a bug needs to be open to be valid
	IsOpen *bool `json:"is_open,omitempty"`
	// TargetRelease determines which release a bug needs to target to be valid
	TargetRelease *string `json:"target_release,omitempty"`
	// Statuses determine which statuses a bug may have to be valid
	Statuses *[]string `json:"statuses,omitempty"`
	// ValidStates determine states in which the bug may be to be valid
	ValidStates *[]BugzillaBugState `json:"valid_states,omitempty"`

	// DependentBugStatuses determine which statuses a bug's dependent bugs may have
	// to deem the child bug valid.  These are merged into DependentBugStates when
	// resolving branch options.
	DependentBugStatuses *[]string `json:"dependent_bug_statuses,omitempty"`
	// DependentBugStates determine states in which a bug's dependents bugs may be
	// to deem the child bug valid.  If set, all blockers must have a valid state.
	DependentBugStates *[]BugzillaBugState `json:"dependent_bug_states,omitempty"`
	// DependentBugTargetReleases determines the set of valid target
	// releases for dependent bugs.  If set, all blockers must have a
	// valid target release.
	DependentBugTargetReleases *[]string `json:"dependent_bug_target_releases,omitempty"`
	// DeprecatedDependentBugTargetRelease determines which release a
	// bug's dependent bugs need to target to be valid.  If set, all
	// blockers must have a valid target releasee.
	//
	// Deprecated: Use DependentBugTargetReleases instead.  If set,
	// DependentBugTargetRelease will be appended to
	// DeprecatedDependentBugTargetReleases.
	DeprecatedDependentBugTargetRelease *string `json:"dependent_bug_target_release,omitempty"`

	// StatusAfterValidation is the status which the bug will be moved to after being
	// deemed valid and linked to a PR. Will implicitly be considered a part of `statuses`
	// if others are set.
	StatusAfterValidation *string `json:"status_after_validation,omitempty"`
	// StateAfterValidation is the state to which the bug will be moved after being
	// deemed valid and linked to a PR. Will implicitly be considered a part of `ValidStates`
	// if others are set.
	StateAfterValidation *BugzillaBugState `json:"state_after_validation,omitempty"`
	// AddExternalLink determines whether the pull request will be added to the Bugzilla
	// bug using the ExternalBug tracker API after being validated
	AddExternalLink *bool `json:"add_external_link,omitempty"`
	// StatusAfterMerge is the status which the bug will be moved to after all pull requests
	// in the external bug tracker have been merged.
	StatusAfterMerge *string `json:"status_after_merge,omitempty"`
	// StateAfterMerge is the state to which the bug will be moved after all pull requests
	// in the external bug tracker have been merged.
	StateAfterMerge *BugzillaBugState `json:"state_after_merge,omitempty"`
}

type BugzillaBugStateSet map[BugzillaBugState]interface{}

func NewBugzillaBugStateSet(states []BugzillaBugState) BugzillaBugStateSet {
	set := make(BugzillaBugStateSet, len(states))
	for _, state := range states {
		set[state] = nil
	}

	return set
}

func (s BugzillaBugStateSet) Has(state BugzillaBugState) bool {
	_, ok := s[state]
	return ok
}

func (s BugzillaBugStateSet) Insert(states ...BugzillaBugState) BugzillaBugStateSet {
	for _, state := range states {
		s[state] = nil
	}
	return s
}

func statesMatch(first, second []BugzillaBugState) bool {
	if len(first) != len(second) {
		return false
	}

	firstSet := NewBugzillaBugStateSet(first)
	secondSet := NewBugzillaBugStateSet(second)

	for state := range firstSet {
		if !secondSet.Has(state) {
			return false
		}
	}

	return true
}

func (o BugzillaBranchOptions) matches(other BugzillaBranchOptions) bool {
	validateByDefaultMatch := o.ValidateByDefault == nil && other.ValidateByDefault == nil ||
		(o.ValidateByDefault != nil && other.ValidateByDefault != nil && *o.ValidateByDefault == *other.ValidateByDefault)
	isOpenMatch := o.IsOpen == nil && other.IsOpen == nil ||
		(o.IsOpen != nil && other.IsOpen != nil && *o.IsOpen == *other.IsOpen)
	targetReleaseMatch := o.TargetRelease == nil && other.TargetRelease == nil ||
		(o.TargetRelease != nil && other.TargetRelease != nil && *o.TargetRelease == *other.TargetRelease)
	bugStatesMatch := o.ValidStates == nil && other.ValidStates == nil ||
		(o.ValidStates != nil && other.ValidStates != nil && statesMatch(*o.ValidStates, *other.ValidStates))
	dependentBugStatesMatch := o.DependentBugStates == nil && other.DependentBugStates == nil ||
		(o.DependentBugStates != nil && other.DependentBugStates != nil && statesMatch(*o.DependentBugStates, *other.DependentBugStates))
	statesAfterValidationMatch := o.StateAfterValidation == nil && other.StateAfterValidation == nil ||
		(o.StateAfterValidation != nil && other.StateAfterValidation != nil && *o.StateAfterValidation == *other.StateAfterValidation)
	addExternalLinkMatch := o.AddExternalLink == nil && other.AddExternalLink == nil ||
		(o.AddExternalLink != nil && other.AddExternalLink != nil && *o.AddExternalLink == *other.AddExternalLink)
	statesAfterMergeMatch := o.StateAfterMerge == nil && other.StateAfterMerge == nil ||
		(o.StateAfterMerge != nil && other.StateAfterMerge != nil && *o.StateAfterMerge == *other.StateAfterMerge)
	return validateByDefaultMatch && isOpenMatch && targetReleaseMatch && bugStatesMatch && dependentBugStatesMatch && statesAfterValidationMatch && addExternalLinkMatch && statesAfterMergeMatch
}

const BugzillaOptionsWildcard = `*`

// OptionsForItem resolves a set of options for an item, honoring
// the `*` wildcard and doing defaulting if it is present with the
// item itself.
func OptionsForItem(item string, config map[string]BugzillaBranchOptions) BugzillaBranchOptions {
	return ResolveBugzillaOptions(config[BugzillaOptionsWildcard], config[item])
}

func mergeStatusesIntoStates(states *[]BugzillaBugState, statuses *[]string) *[]BugzillaBugState {
	var newStates []BugzillaBugState
	stateSet := BugzillaBugStateSet{}

	if states != nil {
		stateSet = stateSet.Insert(*states...)
	}
	if statuses != nil {
		for _, status := range *statuses {
			stateSet = stateSet.Insert(BugzillaBugState{Status: status})
		}
	}

	for state := range stateSet {
		newStates = append(newStates, state)
	}

	if len(newStates) > 0 {
		sort.Slice(newStates, func(i, j int) bool {
			return newStates[i].Status < newStates[j].Status || (newStates[i].Status == newStates[j].Status && newStates[i].Resolution < newStates[j].Resolution)
		})
		return &newStates
	}
	return nil
}

// ResolveBugzillaOptions implements defaulting for a parent/child configuration,
// preferring child fields where set. This method also reflects all "Status"
// fields into matching `State` fields.
func ResolveBugzillaOptions(parent, child BugzillaBranchOptions) BugzillaBranchOptions {
	output := BugzillaBranchOptions{}

	if child.ExcludeDefaults == nil || !*child.ExcludeDefaults {
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
		if parent.ValidStates != nil {
			output.ValidStates = parent.ValidStates
		}
		if parent.Statuses != nil {
			output.Statuses = parent.Statuses
			output.ValidStates = mergeStatusesIntoStates(output.ValidStates, parent.Statuses)
		}
		if parent.DependentBugStates != nil {
			output.DependentBugStates = parent.DependentBugStates
		}
		if parent.DependentBugStatuses != nil {
			output.DependentBugStatuses = parent.DependentBugStatuses
			output.DependentBugStates = mergeStatusesIntoStates(output.DependentBugStates, parent.DependentBugStatuses)
		}
		if parent.DependentBugTargetReleases != nil {
			output.DependentBugTargetReleases = parent.DependentBugTargetReleases
		}
		if parent.DeprecatedDependentBugTargetRelease != nil {
			warnDeprecated(&warnDependentBugTargetRelease, 5*time.Minute, "Please update plugins.yaml to use dependent_bug_target_releases instead of the deprecated dependent_bug_target_release")
			if parent.DependentBugTargetReleases == nil {
				output.DependentBugTargetReleases = &[]string{*parent.DeprecatedDependentBugTargetRelease}
			} else if !sets.NewString(*parent.DependentBugTargetReleases...).Has(*parent.DeprecatedDependentBugTargetRelease) {
				dependentBugTargetReleases := append(*output.DependentBugTargetReleases, *parent.DeprecatedDependentBugTargetRelease)
				output.DependentBugTargetReleases = &dependentBugTargetReleases
			}
		}
		if parent.StatusAfterValidation != nil {
			output.StatusAfterValidation = parent.StatusAfterValidation
			output.StateAfterValidation = &BugzillaBugState{Status: *output.StatusAfterValidation}
		}
		if parent.StateAfterValidation != nil {
			output.StateAfterValidation = parent.StateAfterValidation
		}
		if parent.AddExternalLink != nil {
			output.AddExternalLink = parent.AddExternalLink
		}
		if parent.StatusAfterMerge != nil {
			output.StatusAfterMerge = parent.StatusAfterMerge
			output.StateAfterMerge = &BugzillaBugState{Status: *output.StatusAfterMerge}
		}
		if parent.StateAfterMerge != nil {
			output.StateAfterMerge = parent.StateAfterMerge
		}
	}

	// override with the child
	if child.ValidateByDefault != nil {
		output.ValidateByDefault = child.ValidateByDefault
	}
	if child.IsOpen != nil {
		output.IsOpen = child.IsOpen
	}
	if child.TargetRelease != nil {
		output.TargetRelease = child.TargetRelease
	}

	if child.ValidStates != nil {
		output.ValidStates = child.ValidStates
	}
	if child.Statuses != nil {
		output.Statuses = child.Statuses
		if child.ValidStates == nil {
			output.ValidStates = nil
		}
		output.ValidStates = mergeStatusesIntoStates(output.ValidStates, child.Statuses)
	}

	if child.DependentBugStates != nil {
		output.DependentBugStates = child.DependentBugStates
	}
	if child.DependentBugStatuses != nil {
		output.DependentBugStatuses = child.DependentBugStatuses
		if child.DependentBugStates == nil {
			output.DependentBugStates = nil
		}
		output.DependentBugStates = mergeStatusesIntoStates(output.DependentBugStates, child.DependentBugStatuses)
	}
	if child.DependentBugTargetReleases != nil {
		output.DependentBugTargetReleases = child.DependentBugTargetReleases
	}
	if child.DeprecatedDependentBugTargetRelease != nil {
		warnDeprecated(&warnDependentBugTargetRelease, 5*time.Minute, "Please update plugins.yaml to use dependent_bug_target_releases instead of the deprecated dependent_bug_target_release")
		if child.DependentBugTargetReleases == nil {
			output.DependentBugTargetReleases = &[]string{*child.DeprecatedDependentBugTargetRelease}
		} else if !sets.NewString(*child.DependentBugTargetReleases...).Has(*child.DeprecatedDependentBugTargetRelease) {
			dependentBugTargetReleases := append(*output.DependentBugTargetReleases, *child.DeprecatedDependentBugTargetRelease)
			output.DependentBugTargetReleases = &dependentBugTargetReleases
		}
	}
	if child.StatusAfterValidation != nil {
		output.StatusAfterValidation = child.StatusAfterValidation
		if child.StateAfterValidation == nil {
			output.StateAfterValidation = &BugzillaBugState{Status: *child.StatusAfterValidation}
		}
	}
	if child.StateAfterValidation != nil {
		output.StateAfterValidation = child.StateAfterValidation
	}
	if child.AddExternalLink != nil {
		output.AddExternalLink = child.AddExternalLink
	}
	if child.StatusAfterMerge != nil {
		output.StatusAfterMerge = child.StatusAfterMerge
		if child.StateAfterMerge == nil {
			output.StateAfterMerge = &BugzillaBugState{Status: *child.StatusAfterMerge}
		}
	}
	if child.StateAfterMerge != nil {
		output.StateAfterMerge = child.StateAfterMerge
	}

	// Status fields should not be used anywhere now when they were mirrored to states
	output.Statuses = nil
	output.DependentBugStatuses = nil
	output.StatusAfterMerge = nil
	output.StatusAfterValidation = nil

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

// Override holds options for the override plugin
type Override struct {
	AllowTopLevelOwners bool `json:"allow_top_level_owners,omitempty"`
	// AllowedGitHubTeams is a map of repositories (eg "k/k") to list of GitHub team slugs,
	// members of which are allowed to override contexts
	AllowedGitHubTeams map[string][]string `json:"allowed_github_teams,omitempty"`
}
