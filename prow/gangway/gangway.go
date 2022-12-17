/*
Copyright 2022 The Kubernetes Authors.

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

package gangway

import (
	context "context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	status "google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	prowcrd "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/version"
)

const (
	HEADER_API_CONSUMER_TYPE = "x-endpoint-api-consumer-type"
	HEADER_API_CONSUMER_ID   = "x-endpoint-api-consumer-number"
)

type Gangway struct {
	UnimplementedProwServer
	ConfigAgent              *config.Agent
	ProwJobClient            ProwJobClient
	InRepoConfigCacheHandler *config.InRepoConfigCacheHandler
}

// ProwJobClient is mostly for testing (for calling into the low-level
// Kubernetes API to check whether gangway behaved correctly).
type ProwJobClient interface {
	Create(context.Context, *prowcrd.ProwJob, metav1.CreateOptions) (*prowcrd.ProwJob, error)
}

func (gw *Gangway) CreateJobExecution(ctx context.Context, cjer *CreateJobExecutionRequest) (*JobExecution, error) {
	err, md := getHttpRequestHeaders(ctx)

	if err != nil {
		logrus.WithError(err).Error("could not find request HTTP headers")
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Validate request fields.
	if err := cjer.Validate(); err != nil {
		logrus.WithError(err).Error("could not validate request fields")
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// FIXME (listx) Add execution token generation database call, so that we can
	// reduce the delay between the initial call and the creation of the ProwJob
	// CR. We should probably use UUIDv7 (see
	// https://www.ietf.org/archive/id/draft-peabody-dispatch-new-uuid-format-01.html).
	// Also see FireBase's PushID for comparison:
	// https://firebase.blog/posts/2015/02/the-2120-ways-to-ensure-unique_68.

	// Identify the client from the request metadata.
	mainConfig := gw.ConfigAgent.Config()
	allowedApiClient, err := mainConfig.IdentifyAllowedClient(md)
	if err != nil {
		logrus.WithError(err).Error("could not find client in allowlist")
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	l, err := getDecoratedLoggerEntry(allowedApiClient, md)
	if err != nil {
		l = logrus.NewEntry(logrus.New())
	}

	allowedClusters := []string{"*"}
	var reporterFunc reporterFunc = nil
	requireTenantID := true

	jobExec, err := HandleProwJob(l, reporterFunc, cjer, gw.ProwJobClient, mainConfig, gw.InRepoConfigCacheHandler, allowedApiClient, requireTenantID, allowedClusters)
	if err != nil {
		logrus.WithError(err).Errorf("failed to create job %q", cjer.GetJobName())
		return nil, err
	}

	return jobExec, nil
}

// ClientAuthorized checks whether or not a client can run a Prow job based on
// the job's identifier (is this client allowed to run jobs meant for the given
// identifier?). This needs to traverse the config to determine whether the
// allowlist (allowed_api_clients) allows it.
func ClientAuthorized(allowedApiClient *config.AllowedApiClient, prowJobCR prowcrd.ProwJob) bool {
	pjd := prowJobCR.Spec.ProwJobDefault
	for _, allowedJobsFilter := range allowedApiClient.AllowedJobsFilters {
		if allowedJobsFilter.TenantID == pjd.TenantID {
			return true
		}
	}
	return false
}

// FIXME: Add roundtrip tests to ensure that the conversion between gitRefs and
// refs is lossless.
func ToCrdRefs(gitRefs *Refs) (*prowcrd.Refs, error) {
	if gitRefs == nil {
		return nil, errors.New("gitRefs is nil")
	}

	refs := prowcrd.Refs{
		Org:            gitRefs.Org,
		Repo:           gitRefs.Repo,
		RepoLink:       gitRefs.RepoLink,
		BaseRef:        gitRefs.BaseRef,
		BaseSHA:        gitRefs.BaseSha,
		BaseLink:       gitRefs.BaseLink,
		PathAlias:      gitRefs.PathAlias,
		WorkDir:        gitRefs.WorkDir,
		CloneURI:       gitRefs.CloneUri,
		SkipSubmodules: gitRefs.SkipSubmodules,
		CloneDepth:     int(gitRefs.CloneDepth),
		SkipFetchHead:  gitRefs.SkipFetchHead,
	}

	var pulls []prowcrd.Pull
	for _, pull := range gitRefs.GetPulls() {
		if pull == nil {
			continue
		}
		p := prowcrd.Pull{
			Number:     int(pull.Number),
			Author:     pull.Author,
			SHA:        pull.Sha,
			Title:      pull.Title,
			Ref:        pull.Ref,
			Link:       pull.Link,
			CommitLink: pull.CommitLink,
			AuthorLink: pull.AuthorLink,
		}
		pulls = append(pulls, p)
	}

	refs.Pulls = pulls

	return &refs, nil
}

func FromCrdRefs(refs *prowcrd.Refs) (*Refs, error) {
	if refs == nil {
		return nil, errors.New("refs is nil")
	}

	gitRefs := Refs{
		Org:            refs.Org,
		Repo:           refs.Repo,
		RepoLink:       refs.RepoLink,
		BaseRef:        refs.BaseRef,
		BaseSha:        refs.BaseSHA,
		BaseLink:       refs.BaseLink,
		PathAlias:      refs.PathAlias,
		WorkDir:        refs.WorkDir,
		CloneUri:       refs.CloneURI,
		SkipSubmodules: refs.SkipSubmodules,
		CloneDepth:     int32(refs.CloneDepth),
		SkipFetchHead:  refs.SkipFetchHead,
	}

	var pulls []*Pull
	for _, pull := range refs.Pulls {
		p := Pull{
			Number:     int32(pull.Number),
			Author:     pull.Author,
			Sha:        pull.SHA,
			Title:      pull.Title,
			Ref:        pull.Ref,
			Link:       pull.Link,
			CommitLink: pull.CommitLink,
			AuthorLink: pull.AuthorLink,
		}
		pulls = append(pulls, &p)
	}

	gitRefs.Pulls = pulls

	return &gitRefs, nil
}

func getHttpRequestHeaders(ctx context.Context) (error, *metadata.MD) {
	// Retrieve HTTP headers from call. All headers are lower-cased.
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("error retrieving metadata from context"), nil
	}
	return nil, &md
}

// getDecoratedLoggerEntry turns on a new logger that captures all known
// (interesting) HTTP headers of a gRPC request. We convert these headers into
// log fields so that the logger can be very precise.
func getDecoratedLoggerEntry(allowedApiClient *config.AllowedApiClient, md *metadata.MD) (*logrus.Entry, error) {
	cv, err := allowedApiClient.GetApiClientCloudVendor()
	if err != nil {
		return nil, err
	}

	knownHeaders := cv.GetRequiredMdHeaders()
	fields := make(map[string]interface{})
	for _, header := range knownHeaders {
		values := md.Get(header)
		// Only use the first value. MD stores multiple values in case other
		// entities attempt to overwrite an existing key (it prevents this by
		// storing values as a list of strings).
		//
		// Prefix the field with "http-header/" so that all of the headers here
		// get displayed neatly together (when the fields are sorted by logrus's
		// own output to the console).
		if len(values) > 0 {
			fields[fmt.Sprintf("http-header/%s", header)] = values[0]
		}
	}
	fields["component"] = version.Name

	logrusutil.Init(&logrusutil.DefaultFieldsFormatter{
		PrintLineNumber: true,
		DefaultFields:   fields,
	})

	return logrus.NewEntry(logrus.New()), nil
}

func (cjer *CreateJobExecutionRequest) Validate() error {
	jobName := cjer.GetJobName()
	jobExecutionType := cjer.GetJobExecutionType()
	gitRefs := cjer.GetRefs()

	if len(jobName) == 0 {
		return errors.New("job_name field cannot be empty")
	}

	if jobExecutionType == JobExecutionType_JOB_EXECUTION_TYPE_UNSPECIFIED {
		return fmt.Errorf("unsupported JobExecutionType: %s", jobExecutionType)
	}

	// Periodic jobs are not allowed to be defined with gitRefs. This is because
	// gitRefs can denote inrepoconfig repo information (and periodic jobs are
	// not allowed to be defined via inrepoconfig). See
	// https://github.com/kubernetes/test-infra/issues/21729.
	if jobExecutionType == JobExecutionType_PERIODIC && gitRefs != nil {
		logrus.Error("periodic jobs cannot also have gitRefs")
		return errors.New("periodic jobs cannot also have gitRefs")
	}

	if jobExecutionType != JobExecutionType_PERIODIC {
		// Non-periodic jobs must have a BaseRepo (default repo to clone)
		// defined.
		if gitRefs == nil {
			return fmt.Errorf("gitRefs must be defined for %q", jobExecutionType)
		}
		if err := gitRefs.Validate(); err != nil {
			return fmt.Errorf("gitRefs: failed to validate: %s", err)
		}
	}

	// Finally perform some additional checks on the requested PodSpecOptions.
	podSpecOptions := cjer.GetPodSpecOptions()
	if podSpecOptions != nil {
		envs := podSpecOptions.GetEnvs()
		for k, v := range envs {
			if len(k) == 0 || len(v) == 0 {
				return fmt.Errorf("invalid environment variable key/value pair: %q, %q", k, v)
			}
		}

		labels := podSpecOptions.GetLabels()
		for k, v := range labels {
			if len(k) == 0 || len(v) == 0 {
				return fmt.Errorf("invalid label key/value pair: %q, %q", k, v)
			}

			errs := validation.IsValidLabelValue(v)
			if len(errs) > 0 {
				return fmt.Errorf("invalid label: the following errors found: %q", errs)
			}
		}

		annotations := podSpecOptions.GetAnnotations()
		for k, v := range annotations {
			if len(k) == 0 || len(v) == 0 {
				return fmt.Errorf("invalid annotation key/value pair: %q, %q", k, v)
			}
		}
	}

	return nil
}

func (gitRefs *Refs) Validate() error {
	if len(gitRefs.Org) == 0 {
		return fmt.Errorf("gitRefs: Org cannot be empty")
	}

	if len(gitRefs.Repo) == 0 {
		return fmt.Errorf("gitRefs: Repo cannot be empty")
	}

	if len(gitRefs.BaseRef) == 0 {
		return fmt.Errorf("gitRefs: BaseRef cannot be empty")
	}

	if len(gitRefs.BaseSha) == 0 {
		return fmt.Errorf("gitRefs: BaseSha cannot be empty")
	}

	for _, pull := range gitRefs.Pulls {
		if err := pull.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func (pull *Pull) Validate() error {
	// Commit SHA must be a 40-character hex string.
	var validSha = regexp.MustCompile(`^[0-9a-f]{40}$`)
	if !validSha.MatchString(pull.Sha) {
		return fmt.Errorf("pull: invalid SHA: %q", pull.Sha)
	}
	return nil
}

// Ensure interface is intact. I.e., this declaration ensures that the type
// "*config.Config" implements the "prowCfgClient" interface. See
// https://golang.org/doc/faq#guarantee_satisfies_interface.
var _ prowCfgClient = (*config.Config)(nil)

// prowCfgClient is a subset of all the various behaviors that the
// "*config.Config" type implements, which we will test here.
type prowCfgClient interface {
	AllPeriodics() []config.Periodic
	GetPresubmitsStatic(identifier string) []config.Presubmit
	GetPostsubmitsStatic(identifier string) []config.Postsubmit
	GetProwJobDefault(repo, cluster string) *prowcrd.ProwJobDefault
}

type reporterFunc func(pj *prowcrd.ProwJob, state prowcrd.ProwJobState, err error)

func (cjer *CreateJobExecutionRequest) getJobHandler() (jobHandler, error) {
	var jh jobHandler
	switch cjer.JobExecutionType {
	case JobExecutionType_PERIODIC:
		jh = &periodicJobHandler{}
	case JobExecutionType_PRESUBMIT:
		jh = &presubmitJobHandler{}
	case JobExecutionType_POSTSUBMIT:
		jh = &postsubmitJobHandler{}
	default:
		return nil, fmt.Errorf("unsupported JobExecutionType type: %s", cjer.JobExecutionType)
	}

	return jh, nil
}

// Deep-copy all map fields from a gangway.CreateJobExecutionRequest and also
// the statically defined (configured in YAML) Prow Job labels and annotations.
func mergeMapFields(cjer *CreateJobExecutionRequest, staticLabels, staticAnnotations map[string]string) (map[string]string, map[string]string) {

	pso := cjer.GetPodSpecOptions()

	combinedLabels := make(map[string]string)
	combinedAnnotations := make(map[string]string)

	// Overwrite the static definitions with what we received in the
	// CreateJobExecutionRequest. This order is important.
	for k, v := range staticLabels {
		combinedLabels[k] = v
	}
	for k, v := range pso.GetLabels() {
		combinedLabels[k] = v
	}

	// Do the same for the annotations.
	for k, v := range staticAnnotations {
		combinedAnnotations[k] = v
	}
	for k, v := range pso.GetAnnotations() {
		combinedAnnotations[k] = v
	}

	return combinedLabels, combinedAnnotations
}

func HandleProwJob(l *logrus.Entry,
	reporterFunc reporterFunc,
	cjer *CreateJobExecutionRequest,
	pjc ProwJobClient,
	mainConfig prowCfgClient,
	pc *config.InRepoConfigCacheHandler,
	allowedApiClient *config.AllowedApiClient,
	requireTenantID bool,
	allowedClusters []string) (*JobExecution, error) {

	var prowJobCR prowcrd.ProwJob

	var prowJobSpec *prowcrd.ProwJobSpec
	var jh jobHandler
	jh, err := cjer.getJobHandler()
	if err != nil {
		return nil, err
	}
	prowJobSpec, labels, annotations, err := jh.getProwJobSpec(mainConfig, pc, cjer)
	if err != nil {
		// These are user errors, i.e. missing fields, requested prowjob doesn't exist etc.
		// These errors are already surfaced to user via pubsub two lines below.
		l.WithError(err).WithField("name", cjer.JobName).Info("Failed getting prowjob spec")
		return nil, err
	}
	if prowJobSpec == nil {
		return nil, fmt.Errorf("failed getting prowjob spec") // This should not happen
	}

	combinedLabels, combinedAnnotations := mergeMapFields(cjer, labels, annotations)

	prowJobCR = pjutil.NewProwJob(*prowJobSpec, combinedLabels, combinedAnnotations)
	// Adds / Updates Environments to containers
	if prowJobCR.Spec.PodSpec != nil {
		for i, c := range prowJobCR.Spec.PodSpec.Containers {
			for k, v := range cjer.GetPodSpecOptions().GetEnvs() {
				c.Env = append(c.Env, v1.EnvVar{Name: k, Value: v})
			}
			prowJobCR.Spec.PodSpec.Containers[i].Env = c.Env
		}
	}

	// deny job that runs on not allowed cluster
	var clusterIsAllowed bool
	for _, allowedCluster := range allowedClusters {
		if allowedCluster == "*" || allowedCluster == prowJobSpec.Cluster {
			clusterIsAllowed = true
			break
		}
	}
	// This is a user error, not sure whether we want to return error here.
	if !clusterIsAllowed {
		err := fmt.Errorf("cluster %s is not allowed. Can be fixed by defining this cluster under pubsub_triggers -> allowed_clusters", prowJobSpec.Cluster)
		l.WithField("cluster", prowJobSpec.Cluster).Warn("cluster not allowed")
		if reporterFunc != nil {
			reporterFunc(&prowJobCR, prowcrd.ErrorState, err)
		}
		return nil, err
	}

	// Figure out the tenantID defined for this job by looking it up in its
	// config, or if that's missing, finding the default one specified in the
	// main Config.
	var jobTenantID string
	if prowJobCR.Spec.ProwJobDefault != nil && prowJobCR.Spec.ProwJobDefault.TenantID != "" {
		jobTenantID = prowJobCR.Spec.ProwJobDefault.TenantID
	} else {
		// Derive the orgRepo from the request. Postsubmits and Presubmits both
		// require Git refs information, so we can use that to get the job's
		// associated orgRepo. Then we can feed this orgRepo into
		// mainConfig.GetProwJobDefault(orgRepo, '*') to get the tenantID from
		// the main Config's "prowjob_default_entries" field.
		switch cjer.JobExecutionType {
		case JobExecutionType_POSTSUBMIT:
			fallthrough
		case JobExecutionType_PRESUBMIT:
			orgRepo := fmt.Sprintf("%s/%s", cjer.Refs.Org, cjer.Refs.Repo)
			jobTenantID = mainConfig.GetProwJobDefault(orgRepo, "*").TenantID
		}
	}

	if len(jobTenantID) == 0 {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("could not determine tenant_id for job %s", prowJobCR.Name))
	}
	prowJobCR.Spec.ProwJobDefault.TenantID = jobTenantID

	// Check whether this authenticated API client has authorization to trigger
	// the requested Prow Job.
	if allowedApiClient != nil {
		authorized := ClientAuthorized(allowedApiClient, prowJobCR)

		if !authorized {
			logrus.Error("client is not authorized to execute the given job")
			return nil, status.Error(codes.PermissionDenied, "client is not authorized to execute the given job")
		}
	}

	if _, err := pjc.Create(context.TODO(), &prowJobCR, metav1.CreateOptions{}); err != nil {
		l.WithError(err).Errorf("failed to create job %q as %q", cjer.GetJobName(), prowJobCR.Name)
		if reporterFunc != nil {
			reporterFunc(&prowJobCR, prowcrd.ErrorState, err)
		}
		return nil, err
	}
	l.WithFields(logrus.Fields{
		"job":                 cjer.GetJobName(),
		"name":                prowJobCR.Name,
		"prowjob-annotations": prowJobCR.Annotations,
	}).Info("Job created.")
	if reporterFunc != nil {
		reporterFunc(&prowJobCR, prowcrd.TriggeredState, nil)
	}

	// Now populate a JobExecution. We have to convert data from the ProwJob
	// custom resource to a JobExecution. For now we just reuse the "Name" field
	// of a ProwJob CR as a globally-unique execution ID, because this existing
	// string is already used to do lookups on Deck
	// (https://prow.k8s.io/prowjob?prowjob=c2891365-621c-11ed-88b0-da2d50b4915c)
	// but also for naming the test pod itself (prowcrd.ProwJob.Status.pod_name
	// field).
	jobExec := &JobExecution{
		Id:     cjer.JobName,
		Status: JobExecutionStatus_TRIGGERED,
	}

	return jobExec, nil
}

// jobHandler handles job type specific logic
type jobHandler interface {
	getProwJobSpec(mainConfig prowCfgClient, pc *config.InRepoConfigCacheHandler, cjer *CreateJobExecutionRequest) (prowJobSpec *prowcrd.ProwJobSpec, labels map[string]string, annotations map[string]string, err error)
}

// periodicJobHandler implements jobHandler
type periodicJobHandler struct{}

func (peh *periodicJobHandler) getProwJobSpec(mainConfig prowCfgClient, pc *config.InRepoConfigCacheHandler, cjer *CreateJobExecutionRequest) (prowJobSpec *prowcrd.ProwJobSpec, labels map[string]string, annotations map[string]string, err error) {
	var periodicJob *config.Periodic
	// TODO(chaodaiG): do we want to support inrepoconfig when
	// https://github.com/kubernetes/test-infra/issues/21729 is done?
	for _, job := range mainConfig.AllPeriodics() {
		if job.Name == cjer.JobName {
			// Directly followed by break, so this is ok
			// nolint: exportloopref
			periodicJob = &job
			break
		}
	}
	if periodicJob == nil {
		err = fmt.Errorf("failed to find associated periodic job %q", cjer.JobName)
		return
	}

	spec := pjutil.PeriodicSpec(*periodicJob)
	prowJobSpec = &spec
	labels, annotations = periodicJob.Labels, periodicJob.Annotations
	return
}

// presubmitJobHandler implements jobHandler
type presubmitJobHandler struct {
}

func (prh *presubmitJobHandler) getProwJobSpec(mainConfig prowCfgClient, pc *config.InRepoConfigCacheHandler, cjer *CreateJobExecutionRequest) (prowJobSpec *prowcrd.ProwJobSpec, labels map[string]string, annotations map[string]string, err error) {
	// presubmit jobs require Refs and Refs.Pulls to be set
	refs, err := ToCrdRefs(cjer.Refs)
	if err != nil {
		return
	}
	if refs == nil {
		err = errors.New("Refs must be supplied")
		return
	}
	if len(refs.Org) == 0 {
		err = errors.New("org must be supplied")
		return
	}
	if len(refs.Repo) == 0 {
		err = errors.New("repo must be supplied")
		return
	}
	if len(refs.Pulls) == 0 {
		err = errors.New("at least 1 Pulls is required")
		return
	}
	if len(refs.BaseSHA) == 0 {
		err = errors.New("baseSHA must be supplied")
		return
	}
	if len(refs.BaseRef) == 0 {
		err = errors.New("baseRef must be supplied")
		return
	}

	var presubmitJob *config.Presubmit
	org, repo, branch := refs.Org, refs.Repo, refs.BaseRef
	orgRepo := org + "/" + repo
	// Add "https://" prefix to orgRepo if this is a gerrit job.
	// (Unfortunately gerrit jobs use the full repo URL as the identifier.)
	prefix := "https://"
	if cjer.PodSpecOptions != nil && cjer.PodSpecOptions.Labels[kube.GerritRevision] != "" && !strings.HasPrefix(orgRepo, prefix) {
		orgRepo = prefix + orgRepo
	}
	baseSHAGetter := func() (string, error) {
		return refs.BaseSHA, nil
	}
	var headSHAGetters []func() (string, error)
	for _, pull := range refs.Pulls {
		pull := pull
		headSHAGetters = append(headSHAGetters, func() (string, error) {
			return pull.SHA, nil
		})
	}

	logger := logrus.WithFields(logrus.Fields{"org": org, "repo": repo, "branch": branch, "orgRepo": orgRepo})
	// Get presubmits from Config alone.
	presubmits := mainConfig.GetPresubmitsStatic(orgRepo)
	// If InRepoConfigCache is provided, then it means that we also want to fetch
	// from an inrepoconfig.
	if pc != nil {
		logger.Debug("Getting prow jobs.")
		var presubmitsWithInrepoconfig []config.Presubmit
		var err error
		presubmitsWithInrepoconfig, err = pc.GetPresubmits(orgRepo, baseSHAGetter, headSHAGetters...)
		if err != nil {
			logger.WithError(err).Info("Failed to get presubmits")
		} else {
			logger.WithField("static-jobs", len(presubmits)).WithField("jobs-with-inrepoconfig", len(presubmitsWithInrepoconfig)).Debug("Jobs found.")
			// Overwrite presubmits. This is safe because pc.GetPresubmits()
			// itself calls mainConfig.GetPresubmitsStatic() and adds to it all the
			// presubmits found in the inrepoconfig.
			presubmits = presubmitsWithInrepoconfig
		}
	}

	for _, job := range presubmits {
		job := job
		if !job.CouldRun(branch) { // filter out jobs that are not branch matching
			continue
		}
		if job.Name == cjer.JobName {
			if presubmitJob != nil {
				err = fmt.Errorf("%s matches multiple prow jobs from orgRepo %q", cjer.JobName, orgRepo)
				return
			}
			presubmitJob = &job
		}
	}
	// This also captures the case where fetching jobs from inrepoconfig failed.
	// However doesn't not distinguish between this case and a wrong prow job name.
	if presubmitJob == nil {
		err = fmt.Errorf("failed to find associated presubmit job %q from orgRepo %q", cjer.JobName, orgRepo)
		return
	}

	spec := pjutil.PresubmitSpec(*presubmitJob, *refs)
	prowJobSpec, labels, annotations = &spec, presubmitJob.Labels, presubmitJob.Annotations
	return
}

// postsubmitJobHandler implements jobHandler
type postsubmitJobHandler struct {
}

func (poh *postsubmitJobHandler) getProwJobSpec(mainConfig prowCfgClient, pc *config.InRepoConfigCacheHandler, cjer *CreateJobExecutionRequest) (prowJobSpec *prowcrd.ProwJobSpec, labels map[string]string, annotations map[string]string, err error) {
	// postsubmit jobs require Refs to be set
	refs, err := ToCrdRefs(cjer.Refs)
	if refs == nil {
		err = errors.New("refs must be supplied")
		return
	}
	if len(refs.Org) == 0 {
		err = errors.New("org must be supplied")
		return
	}
	if len(refs.Repo) == 0 {
		err = errors.New("repo must be supplied")
		return
	}
	if len(refs.BaseSHA) == 0 {
		err = errors.New("baseSHA must be supplied")
		return
	}
	if len(refs.BaseRef) == 0 {
		err = errors.New("baseRef must be supplied")
		return
	}

	var postsubmitJob *config.Postsubmit
	org, repo, branch := refs.Org, refs.Repo, refs.BaseRef
	orgRepo := org + "/" + repo
	// Add "https://" prefix to orgRepo if this is a gerrit job.
	// (Unfortunately gerrit jobs use the full repo URL as the identifier.)
	prefix := "https://"
	if cjer.PodSpecOptions != nil && cjer.PodSpecOptions.Labels[kube.GerritRevision] != "" && !strings.HasPrefix(orgRepo, prefix) {
		orgRepo = prefix + orgRepo
	}
	baseSHAGetter := func() (string, error) {
		return refs.BaseSHA, nil
	}

	logger := logrus.WithFields(logrus.Fields{"org": org, "repo": repo, "branch": branch, "orgRepo": orgRepo})
	postsubmits := mainConfig.GetPostsubmitsStatic(orgRepo)
	if pc != nil {
		logger.Debug("Getting prow jobs.")
		var postsubmitsWithInrepoconfig []config.Postsubmit
		var err error
		postsubmitsWithInrepoconfig, err = pc.GetPostsubmits(orgRepo, baseSHAGetter)
		if err != nil {
			logger.WithError(err).Info("Failed to get postsubmits from inrepoconfig")
		} else {
			logger.WithField("static-jobs", len(postsubmits)).WithField("jobs-with-inrepoconfig", len(postsubmitsWithInrepoconfig)).Debug("Jobs found.")
			postsubmits = postsubmitsWithInrepoconfig
		}
	}

	for _, job := range postsubmits {
		job := job
		if !job.CouldRun(branch) { // filter out jobs that are not branch matching
			continue
		}
		if job.Name == cjer.JobName {
			if postsubmitJob != nil {
				return nil, nil, nil, fmt.Errorf("%s matches multiple prow jobs from orgRepo %q", cjer.JobName, orgRepo)
			}
			postsubmitJob = &job
		}
	}
	// This also captures the case where fetching jobs from inrepoconfig failed.
	// However doesn't not distinguish between this case and a wrong prow job name.
	if postsubmitJob == nil {
		err = fmt.Errorf("failed to find associated postsubmit job %q from orgRepo %q", cjer.JobName, orgRepo)
		return
	}

	spec := pjutil.PostsubmitSpec(*postsubmitJob, *refs)
	prowJobSpec, labels, annotations = &spec, postsubmitJob.Labels, postsubmitJob.Annotations
	return
}
