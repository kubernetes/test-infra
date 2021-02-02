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

package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"strings"
	"time"

	pipelinev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowgithub "k8s.io/test-infra/prow/github"
)

// ProwJobType specifies how the job is triggered.
type ProwJobType string

// Various job types.
const (
	// PresubmitJob means it runs on unmerged PRs.
	PresubmitJob ProwJobType = "presubmit"
	// PostsubmitJob means it runs on each new commit.
	PostsubmitJob ProwJobType = "postsubmit"
	// Periodic job means it runs on a time-basis, unrelated to git changes.
	PeriodicJob ProwJobType = "periodic"
	// BatchJob tests multiple unmerged PRs at the same time.
	BatchJob ProwJobType = "batch"
)

// ProwJobState specifies whether the job is running
type ProwJobState string

// Various job states.
const (
	// TriggeredState means the job has been created but not yet scheduled.
	TriggeredState ProwJobState = "triggered"
	// PendingState means the job is currently running and we are waiting for it to finish.
	PendingState ProwJobState = "pending"
	// SuccessState means the job completed without error (exit 0)
	SuccessState ProwJobState = "success"
	// FailureState means the job completed with errors (exit non-zero)
	FailureState ProwJobState = "failure"
	// AbortedState means prow killed the job early (new commit pushed, perhaps).
	AbortedState ProwJobState = "aborted"
	// ErrorState means the job could not schedule (bad config, perhaps).
	ErrorState ProwJobState = "error"
)

// ProwJobAgent specifies the controller (such as plank or jenkins-agent) that runs the job.
type ProwJobAgent string

const (
	// KubernetesAgent means prow will create a pod to run this job.
	KubernetesAgent ProwJobAgent = "kubernetes"
	// JenkinsAgent means prow will schedule the job on jenkins.
	JenkinsAgent ProwJobAgent = "jenkins"
	// TektonAgent means prow will schedule the job via a tekton PipelineRun CRD resource.
	TektonAgent = "tekton-pipeline"
)

const (
	// DefaultClusterAlias specifies the default cluster key to schedule jobs.
	DefaultClusterAlias = "default"
)

const (
	// StartedStatusFile is the JSON file that stores information about the build
	// at the start ob the build. See testgrid/metadata/job.go for more details.
	StartedStatusFile = "started.json"

	// FinishedStatusFile is the JSON file that stores information about the build
	// after its completion. See testgrid/metadata/job.go for more details.
	FinishedStatusFile = "finished.json"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ProwJob contains the spec as well as runtime metadata.
type ProwJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProwJobSpec   `json:"spec,omitempty"`
	Status ProwJobStatus `json:"status,omitempty"`
}

// ProwJobSpec configures the details of the prow job.
//
// Details include the podspec, code to clone, the cluster it runs
// any child jobs, concurrency limitations, etc.
type ProwJobSpec struct {
	// Type is the type of job and informs how
	// the jobs is triggered
	Type ProwJobType `json:"type,omitempty"`
	// Agent determines which controller fulfills
	// this specific ProwJobSpec and runs the job
	Agent ProwJobAgent `json:"agent,omitempty"`
	// Cluster is which Kubernetes cluster is used
	// to run the job, only applicable for that
	// specific agent
	Cluster string `json:"cluster,omitempty"`
	// Namespace defines where to create pods/resources.
	Namespace string `json:"namespace,omitempty"`
	// Job is the name of the job
	Job string `json:"job,omitempty"`
	// Refs is the code under test, determined at
	// runtime by Prow itself
	Refs *Refs `json:"refs,omitempty"`
	// ExtraRefs are auxiliary repositories that
	// need to be cloned, determined from config
	ExtraRefs []Refs `json:"extra_refs,omitempty"`
	// Report determines if the result of this job should
	// be reported (e.g. status on GitHub, message in Slack, etc.)
	Report bool `json:"report,omitempty"`
	// Context is the name of the status context used to
	// report back to GitHub
	Context string `json:"context,omitempty"`
	// RerunCommand is the command a user would write to
	// trigger this job on their pull request
	RerunCommand string `json:"rerun_command,omitempty"`
	// MaxConcurrency restricts the total number of instances
	// of this job that can run in parallel at once
	MaxConcurrency int `json:"max_concurrency,omitempty"`
	// ErrorOnEviction indicates that the ProwJob should be completed and given
	// the ErrorState status if the pod that is executing the job is evicted.
	// If this field is unspecified or false, a new pod will be created to replace
	// the evicted one.
	ErrorOnEviction bool `json:"error_on_eviction,omitempty"`

	// PodSpec provides the basis for running the test under
	// a Kubernetes agent
	PodSpec *corev1.PodSpec `json:"pod_spec,omitempty"`

	// JenkinsSpec holds configuration specific to Jenkins jobs
	JenkinsSpec *JenkinsSpec `json:"jenkins_spec,omitempty"`

	// PipelineRunSpec provides the basis for running the test as
	// a pipeline-crd resource
	// https://github.com/tektoncd/pipeline
	PipelineRunSpec *pipelinev1alpha1.PipelineRunSpec `json:"pipeline_run_spec,omitempty"`

	// DecorationConfig holds configuration options for
	// decorating PodSpecs that users provide
	DecorationConfig *DecorationConfig `json:"decoration_config,omitempty"`

	// ReporterConfig holds reporter-specific configuration
	ReporterConfig *ReporterConfig `json:"reporter_config,omitempty"`

	// RerunAuthConfig holds information about which users can rerun the job
	RerunAuthConfig *RerunAuthConfig `json:"rerun_auth_config,omitempty"`

	// Hidden specifies if the Job is considered hidden.
	// Hidden jobs are only shown by deck instances that have the
	// `--hiddenOnly=true` or `--show-hidden=true` flag set.
	// Presubmits and Postsubmits can also be set to hidden by
	// adding their repository in Decks `hidden_repo` setting.
	Hidden bool `json:"hidden,omitempty"`
}

type GitHubTeamSlug struct {
	Slug string `json:"slug"`
	Org  string `json:"org"`
}

type RerunAuthConfig struct {
	// If AllowAnyone is set to true, any user can rerun the job
	AllowAnyone bool `json:"allow_anyone,omitempty"`
	// GitHubTeams contains IDs of GitHub teams of users who can rerun the job
	// If you know the name of a team and the org it belongs to,
	// you can look up its ID using this command, where the team slug is the hyphenated name:
	// curl -H "Authorization: token <token>" "https://api.github.com/orgs/<org-name>/teams/<team slug>"
	// or, to list all teams in a given org, use
	// curl -H "Authorization: token <token>" "https://api.github.com/orgs/<org-name>/teams"
	GitHubTeamIDs []int `json:"github_team_ids,omitempty"`
	// GitHubTeamSlugs contains slugs and orgs of teams of users who can rerun the job
	GitHubTeamSlugs []GitHubTeamSlug `json:"github_team_slugs,omitempty"`
	// GitHubUsers contains names of individual users who can rerun the job
	GitHubUsers []string `json:"github_users,omitempty"`
	// GitHubOrgs contains names of GitHub organizations whose members can rerun the job
	GitHubOrgs []string `json:"github_orgs,omitempty"`
}

// IsSpecifiedUser returns true if AllowAnyone is set to true or if the given user is
// specified as a permitted GitHubUser
func (rac *RerunAuthConfig) IsAuthorized(org, user string, cli prowgithub.RerunClient) (bool, error) {
	if rac == nil {
		return false, nil
	}
	if rac.AllowAnyone {
		return true, nil
	}
	for _, u := range rac.GitHubUsers {
		if prowgithub.NormLogin(u) == prowgithub.NormLogin(user) {
			return true, nil
		}
	}
	// if there is no client, no token was provided, so we cannot access the teams
	if cli == nil {
		return false, nil
	}
	for _, gho := range rac.GitHubOrgs {
		isOrgMember, err := cli.IsMember(gho, user)
		if err != nil {
			return false, fmt.Errorf("GitHub failed to fetch members of org %v: %v", gho, err)
		}
		if isOrgMember {
			return true, nil
		}
	}
	for _, ght := range rac.GitHubTeamIDs {
		member, err := cli.TeamHasMember(org, ght, user)
		if err != nil {
			return false, fmt.Errorf("GitHub failed to fetch members of team %v, verify that you have the correct team number and access token: %v", ght, err)
		}
		if member {
			return true, nil
		}
	}
	for _, ghts := range rac.GitHubTeamSlugs {
		team, err := cli.GetTeamBySlug(ghts.Slug, ghts.Org)
		if err != nil {
			return false, fmt.Errorf("GitHub failed to fetch team with slug %s and org %s: %v", ghts.Slug, ghts.Org, err)
		}
		member, err := cli.TeamHasMember(org, team.ID, user)
		if err != nil {
			return false, fmt.Errorf("GitHub failed to fetch members of team %v: %v", team, err)
		}
		if member {
			return true, nil
		}
	}
	return false, nil
}

// Validate validates the RerunAuthConfig fields.
func (rac *RerunAuthConfig) Validate() error {
	if rac == nil {
		return nil
	}

	hasAllowList := len(rac.GitHubUsers) > 0 || len(rac.GitHubTeamIDs) > 0 || len(rac.GitHubTeamSlugs) > 0 || len(rac.GitHubOrgs) > 0

	// If an allowlist is specified, the user probably does not intend for anyone to be able to rerun any job.
	if rac.AllowAnyone && hasAllowList {
		return errors.New("allow anyone is set to true and permitted users or groups are specified")
	}

	return nil
}

// IsAllowAnyone checks if anyone can rerun the job.
func (rac *RerunAuthConfig) IsAllowAnyone() bool {
	if rac == nil {
		return false
	}

	return rac.AllowAnyone
}

type ReporterConfig struct {
	Slack *SlackReporterConfig `json:"slack,omitempty"`
}

type SlackReporterConfig struct {
	Channel           string         `json:"channel,omitempty"`
	JobStatesToReport []ProwJobState `json:"job_states_to_report,omitempty"`
	ReportTemplate    string         `json:"report_template,omitempty"`
}

// Duration is a wrapper around time.Duration that parses times in either
// 'integer number of nanoseconds' or 'duration string' formats and serializes
// to 'duration string' format.
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &d.Duration); err == nil {
		// b was an integer number of nanoseconds.
		return nil
	}
	// b was not an integer. Assume that it is a duration string.

	var str string
	err := json.Unmarshal(b, &str)
	if err != nil {
		return err
	}

	pd, err := time.ParseDuration(str)
	if err != nil {
		return err
	}
	d.Duration = pd
	return nil
}

func (d *Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

// DecorationConfig specifies how to augment pods.
//
// This is primarily used to provide automatic integration with gubernator
// and testgrid.
type DecorationConfig struct {
	// Timeout is how long the pod utilities will wait
	// before aborting a job with SIGINT.
	Timeout *Duration `json:"timeout,omitempty"`
	// GracePeriod is how long the pod utilities will wait
	// after sending SIGINT to send SIGKILL when aborting
	// a job. Only applicable if decorating the PodSpec.
	GracePeriod *Duration `json:"grace_period,omitempty"`

	// UtilityImages holds pull specs for utility container
	// images used to decorate a PodSpec.
	UtilityImages *UtilityImages `json:"utility_images,omitempty"`
	// Resources holds resource requests and limits for utility
	// containers used to decorate a PodSpec.
	Resources *Resources `json:"resources,omitempty"`
	// GCSConfiguration holds options for pushing logs and
	// artifacts to GCS from a job.
	GCSConfiguration *GCSConfiguration `json:"gcs_configuration,omitempty"`
	// GCSCredentialsSecret is the name of the Kubernetes secret
	// that holds GCS push credentials.
	GCSCredentialsSecret *string `json:"gcs_credentials_secret,omitempty"`
	// S3CredentialsSecret is the name of the Kubernetes secret
	// that holds blob storage push credentials.
	S3CredentialsSecret *string `json:"s3_credentials_secret,omitempty"`
	// DefaultServiceAccountName is the name of the Kubernetes service account
	// that should be used by the pod if one is not specified in the podspec.
	DefaultServiceAccountName *string `json:"default_service_account_name,omitempty"`
	// SSHKeySecrets are the names of Kubernetes secrets that contain
	// SSK keys which should be used during the cloning process.
	SSHKeySecrets []string `json:"ssh_key_secrets,omitempty"`
	// SSHHostFingerprints are the fingerprints of known SSH hosts
	// that the cloning process can trust.
	// Create with ssh-keyscan [-t rsa] host
	SSHHostFingerprints []string `json:"ssh_host_fingerprints,omitempty"`
	// SkipCloning determines if we should clone source code in the
	// initcontainers for jobs that specify refs
	SkipCloning *bool `json:"skip_cloning,omitempty"`
	// CookieFileSecret is the name of a kubernetes secret that contains
	// a git http.cookiefile, which should be used during the cloning process.
	CookiefileSecret string `json:"cookiefile_secret,omitempty"`
	// OauthTokenSecret is a Kubernetes secret that contains the OAuth token,
	// which is going to be used for fetching a private repository.
	OauthTokenSecret *OauthTokenSecret `json:"oauth_token_secret,omitempty"`
}

// Resources holds resource requests and limits for
// containers used to decorate a PodSpec
type Resources struct {
	CloneRefs       *corev1.ResourceRequirements `json:"clonerefs,omitempty"`
	InitUpload      *corev1.ResourceRequirements `json:"initupload,omitempty"`
	PlaceEntrypoint *corev1.ResourceRequirements `json:"place_entrypoint,omitempty"`
	Sidecar         *corev1.ResourceRequirements `json:"sidecar,omitempty"`
}

// ApplyDefault applies the defaults for the resource decorations. If a field has a zero value,
// it replaces that with the value set in def.
func (u *Resources) ApplyDefault(def *Resources) *Resources {
	if u == nil {
		return def
	} else if def == nil {
		return u
	}

	merged := *u
	if merged.CloneRefs == nil {
		merged.CloneRefs = def.CloneRefs
	}
	if merged.InitUpload == nil {
		merged.InitUpload = def.InitUpload
	}
	if merged.PlaceEntrypoint == nil {
		merged.PlaceEntrypoint = def.PlaceEntrypoint
	}
	if merged.Sidecar == nil {
		merged.Sidecar = def.Sidecar
	}
	return &merged
}

// OauthTokenSecret holds the information of the oauth token's secret name and key.
type OauthTokenSecret struct {
	// Name is the name of a kubernetes secret.
	Name string `json:"name,omitempty"`
	// Key is the a key of the corresponding kubernetes secret that
	// holds the value of the OAuth token.
	Key string `json:"key,omitempty"`
}

// ApplyDefault applies the defaults for the ProwJob decoration. If a field has a zero value, it
// replaces that with the value set in def.
func (d *DecorationConfig) ApplyDefault(def *DecorationConfig) *DecorationConfig {
	if d == nil && def == nil {
		return nil
	}
	var merged DecorationConfig
	if d != nil {
		merged = *d.DeepCopy()
	} else {
		merged = *def.DeepCopy()
	}
	if d == nil || def == nil {
		return &merged
	}
	merged.UtilityImages = merged.UtilityImages.ApplyDefault(def.UtilityImages)
	merged.Resources = merged.Resources.ApplyDefault(def.Resources)
	merged.GCSConfiguration = merged.GCSConfiguration.ApplyDefault(def.GCSConfiguration)

	if merged.Timeout == nil {
		merged.Timeout = def.Timeout
	}
	if merged.GracePeriod == nil {
		merged.GracePeriod = def.GracePeriod
	}
	if merged.GCSCredentialsSecret == nil {
		merged.GCSCredentialsSecret = def.GCSCredentialsSecret
	}
	if merged.S3CredentialsSecret == nil {
		merged.S3CredentialsSecret = def.S3CredentialsSecret
	}
	if merged.DefaultServiceAccountName == nil {
		merged.DefaultServiceAccountName = def.DefaultServiceAccountName
	}
	if len(merged.SSHKeySecrets) == 0 {
		merged.SSHKeySecrets = def.SSHKeySecrets
	}
	if len(merged.SSHHostFingerprints) == 0 {
		merged.SSHHostFingerprints = def.SSHHostFingerprints
	}
	if merged.SkipCloning == nil {
		merged.SkipCloning = def.SkipCloning
	}
	if merged.CookiefileSecret == "" {
		merged.CookiefileSecret = def.CookiefileSecret
	}
	if merged.OauthTokenSecret == nil {
		merged.OauthTokenSecret = def.OauthTokenSecret
	}

	return &merged
}

// Validate ensures all the values set in the DecorationConfig are valid.
func (d *DecorationConfig) Validate() error {
	if d.UtilityImages == nil {
		return errors.New("utility image config is not specified")
	}
	var missing []string
	if d.UtilityImages.CloneRefs == "" {
		missing = append(missing, "clonerefs")
	}
	if d.UtilityImages.InitUpload == "" {
		missing = append(missing, "initupload")
	}
	if d.UtilityImages.Entrypoint == "" {
		missing = append(missing, "entrypoint")
	}
	if d.UtilityImages.Sidecar == "" {
		missing = append(missing, "sidecar")
	}
	if len(missing) > 0 {
		return fmt.Errorf("the following utility images are not specified: %q", missing)
	}

	if d.GCSConfiguration == nil {
		return errors.New("GCS upload configuration is not specified")
	}
	// Intentionally allow d.GCSCredentialsSecret and d.S3CredentialsSecret to
	// be unset in which case we assume GCS permissions are provided by GKE
	// Workload Identity: https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity

	if err := d.GCSConfiguration.Validate(); err != nil {
		return fmt.Errorf("GCS configuration is invalid: %v", err)
	}
	if d.OauthTokenSecret != nil && len(d.SSHKeySecrets) > 0 {
		return errors.New("both OAuth token and SSH key secrets are specified")
	}
	return nil
}

func (d *Duration) Get() time.Duration {
	if d == nil {
		return 0
	}
	return d.Duration
}

// UtilityImages holds pull specs for the utility images
// to be used for a job
type UtilityImages struct {
	// CloneRefs is the pull spec used for the clonerefs utility
	CloneRefs string `json:"clonerefs,omitempty"`
	// InitUpload is the pull spec used for the initupload utility
	InitUpload string `json:"initupload,omitempty"`
	// Entrypoint is the pull spec used for the entrypoint utility
	Entrypoint string `json:"entrypoint,omitempty"`
	// sidecar is the pull spec used for the sidecar utility
	Sidecar string `json:"sidecar,omitempty"`
}

// ApplyDefault applies the defaults for the UtilityImages decorations. If a field has a zero value,
// it replaces that with the value set in def.
func (u *UtilityImages) ApplyDefault(def *UtilityImages) *UtilityImages {
	if u == nil {
		return def
	} else if def == nil {
		return u
	}

	merged := *u.DeepCopy()
	if merged.CloneRefs == "" {
		merged.CloneRefs = def.CloneRefs
	}
	if merged.InitUpload == "" {
		merged.InitUpload = def.InitUpload
	}
	if merged.Entrypoint == "" {
		merged.Entrypoint = def.Entrypoint
	}
	if merged.Sidecar == "" {
		merged.Sidecar = def.Sidecar
	}
	return &merged
}

// PathStrategy specifies minutia about how to construct the url.
// Usually consumed by gubernator/testgrid.
const (
	PathStrategyLegacy   = "legacy"
	PathStrategySingle   = "single"
	PathStrategyExplicit = "explicit"
)

// GCSConfiguration holds options for pushing logs and
// artifacts to GCS from a job.
type GCSConfiguration struct {
	// Bucket is the bucket to upload to, it can be:
	// * a GCS bucket: with gs:// prefix
	// * a S3 bucket: with s3:// prefix
	// * a GCS bucket: without a prefix (deprecated, it's discouraged to use Bucket without prefix please add the gs:// prefix)
	Bucket string `json:"bucket,omitempty"`
	// PathPrefix is an optional path that follows the
	// bucket name and comes before any structure
	PathPrefix string `json:"path_prefix,omitempty"`
	// PathStrategy dictates how the org and repo are used
	// when calculating the full path to an artifact in GCS
	PathStrategy string `json:"path_strategy,omitempty"`
	// DefaultOrg is omitted from GCS paths when using the
	// legacy or simple strategy
	DefaultOrg string `json:"default_org,omitempty"`
	// DefaultRepo is omitted from GCS paths when using the
	// legacy or simple strategy
	DefaultRepo string `json:"default_repo,omitempty"`
	// MediaTypes holds additional extension media types to add to Go's
	// builtin's and the local system's defaults.  This maps extensions
	// to media types, for example: MediaTypes["log"] = "text/plain"
	MediaTypes map[string]string `json:"mediaTypes,omitempty"`
	// JobURLPrefix holds the baseURL under which the jobs output can be viewed.
	// If unset, this will be derived based on org/repo from the job_url_prefix_config.
	JobURLPrefix string `json:"job_url_prefix,omitempty"`

	// LocalOutputDir specifies a directory where files should be copied INSTEAD of uploading to blob storage.
	// This option is useful for testing jobs that use the pod-utilities without actually uploading.
	LocalOutputDir string `json:"local_output_dir,omitempty"`
}

// ApplyDefault applies the defaults for GCSConfiguration decorations. If a field has a zero value,
// it replaces that with the value set in def.
func (g *GCSConfiguration) ApplyDefault(def *GCSConfiguration) *GCSConfiguration {
	if g == nil && def == nil {
		return nil
	}
	var merged GCSConfiguration
	if g != nil {
		merged = *g.DeepCopy()
	} else {
		merged = *def.DeepCopy()
	}
	if g == nil || def == nil {
		return &merged
	}

	if merged.Bucket == "" {
		merged.Bucket = def.Bucket
	}
	if merged.PathPrefix == "" {
		merged.PathPrefix = def.PathPrefix
	}
	if merged.PathStrategy == "" {
		merged.PathStrategy = def.PathStrategy
	}
	if merged.DefaultOrg == "" {
		merged.DefaultOrg = def.DefaultOrg
	}
	if merged.DefaultRepo == "" {
		merged.DefaultRepo = def.DefaultRepo
	}

	if merged.MediaTypes == nil && len(def.MediaTypes) > 0 {
		merged.MediaTypes = map[string]string{}
	}

	for extension, mediaType := range def.MediaTypes {
		merged.MediaTypes[extension] = mediaType
	}

	if merged.JobURLPrefix == "" {
		merged.JobURLPrefix = def.JobURLPrefix
	}

	if merged.LocalOutputDir == "" {
		merged.LocalOutputDir = def.LocalOutputDir
	}
	return &merged
}

// Validate ensures all the values set in the GCSConfiguration are valid.
func (g *GCSConfiguration) Validate() error {
	if _, err := ParsePath(g.Bucket); err != nil {
		return err
	}
	for _, mediaType := range g.MediaTypes {
		if _, _, err := mime.ParseMediaType(mediaType); err != nil {
			return fmt.Errorf("invalid extension media type %q: %v", mediaType, err)
		}
	}
	if g.PathStrategy != PathStrategyLegacy && g.PathStrategy != PathStrategyExplicit && g.PathStrategy != PathStrategySingle {
		return fmt.Errorf("gcs_path_strategy must be one of %q, %q, or %q", PathStrategyLegacy, PathStrategyExplicit, PathStrategySingle)
	}
	if g.PathStrategy != PathStrategyExplicit && (g.DefaultOrg == "" || g.DefaultRepo == "") {
		return fmt.Errorf("default org and repo must be provided for GCS strategy %q", g.PathStrategy)
	}
	return nil
}

type ProwPath url.URL

func (pp ProwPath) StorageProvider() string {
	return pp.Scheme
}

func (pp ProwPath) Bucket() string {
	return pp.Host
}

func (pp ProwPath) FullPath() string {
	return pp.Host + pp.Path
}

// ParsePath tries to extract the ProwPath from, e.g.:
// * <bucket-name> (storageProvider gs)
// * <storage-provider>://<bucket-name>
func ParsePath(bucket string) (*ProwPath, error) {
	// default to GCS if no storage-provider is specified
	if !strings.Contains(bucket, "://") {
		bucket = "gs://" + bucket
	}
	parsedBucket, err := url.Parse(bucket)
	if err != nil {
		return nil, fmt.Errorf("path %q has invalid format, expected either <bucket-name>[/<path>] or <storage-provider>://<bucket-name>[/<path>]", bucket)
	}
	pp := ProwPath(*parsedBucket)
	return &pp, nil
}

// ProwJobStatus provides runtime metadata, such as when it finished, whether it is running, etc.
type ProwJobStatus struct {
	// StartTime is equal to the creation time of the ProwJob
	StartTime metav1.Time `json:"startTime,omitempty"`
	// PendingTime is the timestamp for when the job moved from triggered to pending
	PendingTime *metav1.Time `json:"pendingTime,omitempty"`
	// CompletionTime is the timestamp for when the job goes to a final state
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	State          ProwJobState `json:"state,omitempty"`
	Description    string       `json:"description,omitempty"`
	URL            string       `json:"url,omitempty"`

	// PodName applies only to ProwJobs fulfilled by
	// plank. This field should always be the same as
	// the ProwJob.ObjectMeta.Name field.
	PodName string `json:"pod_name,omitempty"`

	// BuildID is the build identifier vended either by tot
	// or the snowflake library for this job and used as an
	// identifier for grouping artifacts in GCS for views in
	// TestGrid and Gubernator. Idenitifiers vended by tot
	// are monotonically increasing whereas identifiers vended
	// by the snowflake library are not.
	BuildID string `json:"build_id,omitempty"`

	// JenkinsBuildID applies only to ProwJobs fulfilled
	// by the jenkins-operator. This field is the build
	// identifier that Jenkins gave to the build for this
	// ProwJob.
	JenkinsBuildID string `json:"jenkins_build_id,omitempty"`

	// PrevReportStates stores the previous reported prowjob state per reporter
	// So crier won't make duplicated report attempt
	PrevReportStates map[string]ProwJobState `json:"prev_report_states,omitempty"`
}

// Complete returns true if the prow job has finished
func (j *ProwJob) Complete() bool {
	// TODO(fejta): support a timeout?
	return j.Status.CompletionTime != nil
}

// SetComplete marks the job as completed (at time now).
func (j *ProwJob) SetComplete() {
	j.Status.CompletionTime = new(metav1.Time)
	*j.Status.CompletionTime = metav1.Now()
}

// ClusterAlias specifies the key in the clusters map to use.
//
// This allows scheduling a prow job somewhere aside from the default build cluster.
func (j *ProwJob) ClusterAlias() string {
	if j.Spec.Cluster == "" {
		return DefaultClusterAlias
	}
	return j.Spec.Cluster
}

// Pull describes a pull request at a particular point in time.
type Pull struct {
	Number int    `json:"number"`
	Author string `json:"author"`
	SHA    string `json:"sha"`
	Title  string `json:"title,omitempty"`

	// Ref is git ref can be checked out for a change
	// for example,
	// github: pull/123/head
	// gerrit: refs/changes/00/123/1
	Ref string `json:"ref,omitempty"`
	// Link links to the pull request itself.
	Link string `json:"link,omitempty"`
	// CommitLink links to the commit identified by the SHA.
	CommitLink string `json:"commit_link,omitempty"`
	// AuthorLink links to the author of the pull request.
	AuthorLink string `json:"author_link,omitempty"`
}

// Refs describes how the repo was constructed.
type Refs struct {
	// Org is something like kubernetes or k8s.io
	Org string `json:"org"`
	// Repo is something like test-infra
	Repo string `json:"repo"`
	// RepoLink links to the source for Repo.
	RepoLink string `json:"repo_link,omitempty"`

	BaseRef string `json:"base_ref,omitempty"`
	BaseSHA string `json:"base_sha,omitempty"`
	// BaseLink is a link to the commit identified by BaseSHA.
	BaseLink string `json:"base_link,omitempty"`

	Pulls []Pull `json:"pulls,omitempty"`

	// PathAlias is the location under <root-dir>/src
	// where this repository is cloned. If this is not
	// set, <root-dir>/src/github.com/org/repo will be
	// used as the default.
	PathAlias string `json:"path_alias,omitempty"`

	// WorkDir defines if the location of the cloned
	// repository will be used as the default working
	// directory.
	WorkDir bool `json:"workdir,omitempty"`

	// CloneURI is the URI that is used to clone the
	// repository. If unset, will default to
	// `https://github.com/org/repo.git`.
	CloneURI string `json:"clone_uri,omitempty"`
	// SkipSubmodules determines if submodules should be
	// cloned when the job is run. Defaults to true.
	SkipSubmodules bool `json:"skip_submodules,omitempty"`
	// CloneDepth is the depth of the clone that will be used.
	// A depth of zero will do a full clone.
	CloneDepth int `json:"clone_depth,omitempty"`
	// SkipFetchHead tells prow to avoid a git fetch <remote> call.
	// Multiheaded repos may need to not make this call.
	// The git fetch <remote> <BaseRef> call occurs regardless.
	SkipFetchHead bool `json:"skip_fetch_head,omitempty"`
}

func (r Refs) String() string {
	rs := []string{}
	if r.BaseSHA != "" {
		rs = append(rs, fmt.Sprintf("%s:%s", r.BaseRef, r.BaseSHA))
	} else {
		rs = append(rs, r.BaseRef)
	}

	for _, pull := range r.Pulls {
		ref := fmt.Sprintf("%d:%s", pull.Number, pull.SHA)

		if pull.Ref != "" {
			ref = fmt.Sprintf("%s:%s", ref, pull.Ref)
		}

		rs = append(rs, ref)
	}
	return strings.Join(rs, ",")
}

// JenkinsSpec is optional parameters for Jenkins jobs.
// Currently, the only parameter supported is for telling
// jenkins-operator that the job is generated by the https://go.cloudbees.com/docs/plugins/github-branch-source/#github-branch-source plugin
type JenkinsSpec struct {
	GitHubBranchSourceJob bool `json:"github_branch_source_job,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ProwJobList is a list of ProwJob resources
type ProwJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ProwJob `json:"items"`
}
