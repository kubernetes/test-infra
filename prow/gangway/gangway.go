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

	"github.com/sirupsen/logrus"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	status "google.golang.org/grpc/status"
	coreapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	prowcrd "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
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

func (gw *Gangway) CreateJobExecution(ctx context.Context, req *CreateJobExecutionRequest) (*JobExecution, error) {
	err, md := getHttpRequestHeaders(ctx)
	if err != nil {
		logrus.WithError(err).Error("could not find request HTTP headers")
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Validate request fields.
	if err := req.Validate(); err != nil {
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

	enableDecoratedLogger(allowedApiClient, md)

	// Fetch the job definition.
	// FIXME: Refactor this after refactoring Sub.
	jobStruct := JobStruct{}
	/*
		jobStruct, err := gw.FetchJobStruct(req)
		if err != nil {
			logrus.WithError(err).Error("could not find requested job config")
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	*/
	var periodic config.Periodic
	var presubmit config.Presubmit
	var postsubmit config.Postsubmit
	var spec prowcrd.ProwJobSpec
	var refs *prowcrd.Refs

	// Construct "prowcrd.Refs" type which encodes the Git references we want to
	// clone/merge at runtime. We get this information from the request, as well
	// as the Periodic/Presubmut/Postsubmit struct's JobBase.ExtraRefs (which
	// has type prowcrd.Refs).
	gitRefs := req.GetRefs()
	refs, err = ToCrdRefs(gitRefs)
	if err != nil {
		logrus.WithError(err).Error("could not construct refs from baseRepo")
		return nil, err
	}

	// Coerce jobStruct into either a Presubmit, Postsubmit, or Periodic type, based on the
	switch jobStruct.JobExecutionType {
	case JobExecutionType_PERIODIC:
		periodic = *jobStruct.Periodic
		// We don't allow periodic jobs to clone a base repo. This is mainly
		// because we're using the underlying pjutil.PeriodicSpec() function
		// which doesn't take a "refs" argument.
		spec = pjutil.PeriodicSpec(periodic)
	case JobExecutionType_POSTSUBMIT:
		postsubmit = *jobStruct.Postsubmit
		spec = pjutil.PostsubmitSpec(postsubmit, *refs)
	case JobExecutionType_PRESUBMIT:
		presubmit = *jobStruct.Presubmit
		spec = pjutil.PresubmitSpec(presubmit, *refs)
	}

	// Inject labels, annotations, and envs into the job.
	podSpecOptions := req.GetPodSpecOptions()
	labels := make(map[string]string)
	annotations := make(map[string]string)
	if podSpecOptions != nil {
		psoLabels := podSpecOptions.GetLabels()
		for k, v := range psoLabels {
			labels[k] = v
		}
		psoAnnotations := podSpecOptions.GetAnnotations()
		for k, v := range psoAnnotations {
			annotations[k] = v
		}
	}
	prowJobCR := pjutil.NewProwJob(spec, labels, annotations)
	if prowJobCR.Spec.PodSpec != nil {
		if podSpecOptions != nil {
			envs := podSpecOptions.GetEnvs()
			if envs != nil {
				for i, c := range prowJobCR.Spec.PodSpec.Containers {
					for k, v := range envs {
						c.Env = append(c.Env, coreapi.EnvVar{Name: k, Value: v})
					}
					prowJobCR.Spec.PodSpec.Containers[i].Env = c.Env
				}
			}
		}
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
		switch jobStruct.JobExecutionType {
		case JobExecutionType_POSTSUBMIT:
			fallthrough
		case JobExecutionType_PRESUBMIT:
			orgRepo := fmt.Sprintf("%s/%s", refs.Org, refs.Repo)
			jobTenantID = mainConfig.GetProwJobDefault(orgRepo, "*").TenantID
		}
	}

	if len(jobTenantID) == 0 {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("could not determine tenant_id for job %s", prowJobCR.Name))
	}
	prowJobCR.Spec.ProwJobDefault.TenantID = jobTenantID

	// At this point we know that this request is authorized (the request has
	// GCP-specific headers, and the headers point to an allowlisted client ID).
	// Now we need to check whether this authenticated API client has
	// authorization to trigger the requested Prow Job.
	authorized := ClientAuthorized(allowedApiClient, prowJobCR)

	if !authorized {
		logrus.Error("client is not authorized to execute the given job")
		return nil, status.Error(codes.PermissionDenied, "client is not authorized to execute the given job")
	}

	if _, err := gw.ProwJobClient.Create(context.TODO(), &prowJobCR, metav1.CreateOptions{}); err != nil {
		logrus.WithError(err).Errorf("failed to create job %q as %q", req.GetJobName(), prowJobCR.Name)
		return nil, err
	} else {
		logrus.Infof("created Prow Job %s", prowJobCR.Name)
	}

	// Now populate a JobExecution. We have to convert data from the ProwJob
	// custom resource to a JobExecution. For now we just reuse the "Name" field
	// of a ProwJob CR as a globally-unique execution ID, because this existing
	// string is already used to do lookups on Deck
	// (https://prow.k8s.io/prowjob?prowjob=c2891365-621c-11ed-88b0-da2d50b4915c)
	// but also for naming the test pod itself (prowcrd.ProwJob.Status.pod_name
	// field).
	return &JobExecution{
		Id:     prowJobCR.Name,
		Status: JobExecutionStatus_TRIGGERED,
	}, nil
}

// ClientAuthorized checks whether or not a client can run a Prow job based on
// the job's identifier (is this client allowed to run jobs meant for the given
// identifier?). This needs to traverse the config to determine whether the
// allowlist (allowed_api_clients) allows it.
func ClientAuthorized(allowedApiClient *config.AllowedApiClient, prowJobCR prowcrd.ProwJob) bool {
	pjd := prowJobCR.Spec.ProwJobDefault
	for _, allowedJobSubset := range allowedApiClient.AllowedJobSubsets {
		if allowedJobSubset.TenantID == pjd.TenantID {
			return true
		}
	}
	return false
}

// FIXME: Add roundtrip tests to ensure that the conversion between gitRefs and refs is lossless.
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

type JobStruct struct {
	Periodic   *config.Periodic
	Postsubmit *config.Postsubmit
	Presubmit  *config.Presubmit
	// Encode the type of the Job to coerce into (via type assertion), so that
	// users of JobStruct know how to make sense of the Job details.
	JobExecutionType JobExecutionType
}

func getHttpRequestHeaders(ctx context.Context) (error, *metadata.MD) {
	// Retrieve HTTP headers from call. All headers are lower-cased.
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("error retrieving metadata from context"), nil
	}
	return nil, &md
}

// enableDecoratedLogger turns on a new logger that captures all known
// (interesting) HTTP headers of a gRPC request. We convert these headers into
// log fields so that the logger can be very precise.
func enableDecoratedLogger(allowedApiClient *config.AllowedApiClient, md *metadata.MD) {
	knownHeaders := []string{}
	if allowedApiClient.Id.GCP != nil {
		// These headers were drawn from this example:
		// https://github.com/envoyproxy/envoy/issues/13207 (source code appears
		// to be
		// https://github.com/GoogleCloudPlatform/esp-v2/blob/3828042e5b3f840e17837c1a019f4014276014d8/tests/endpoints/bookstore_grpc/server/server.go).
		// Here's an example of what these headers can look like in practice
		// (whitespace edited for readability):
		//
		//     map[
		//       :authority:[localhost:20785]
		//       accept-encoding:[gzip]
		//       content-type:[application/grpc]
		//       user-agent:[Go-http-client/1.1]
		//       x-endpoint-api-consumer-number:[123456]
		//       x-endpoint-api-consumer-type:[PROJECT]
		//       x-envoy-original-method:[GET]
		//       x-envoy-original-path:[/v1/shelves/200?key=api-key]
		//       x-forwarded-proto:[http]
		//       x-request-id:[44770c9a-ee5f-4e36-944e-198b8d9c5196]
		//       ]
		knownHeaders = []string{
			":authority",
			"user-agent",
			"x-endpoint-api-consumer-number",
			"x-endpoint-api-consumer-type",
			"x-envoy-original-method",
			"x-envoy-original-path",
			"x-forwarded-proto",
			"x-request-id",
		}
	}
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
}

func (req *CreateJobExecutionRequest) Validate() error {
	jobName := req.GetJobName()
	jobExecutionType := req.GetJobExecutionType()
	gitRefs := req.GetRefs()

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

	// FIXME: Add validation for Refs field.

	/*

		// Check whether the gitRefs looks correct on the surface (this
		// is a cursory check only, but still worth doing).
		if gitRefs != nil {
			base := gitRefs.GetBase()
			if base == nil {
				return errors.New("gitRefs: base repo cannot be nil")
			}

			// Check whether the base repo exists (this is required).
			if err := base.Validate(); err != nil {
				return fmt.Errorf("invalid base repo for gitRefs: %s", err)
			}

			// It could be that the job definition is only defined in a GitHub Pull
			// Request or Gerrit Change. So in order to get that job definition we
			// have to merge in the PR or Change.
			//
			// Technically a PR will always only have a single "ref" or head SHA
			// commit. However the data structure we have here is for a list of
			// refs, because we are leaving it possible to request batch jobs (which
			// merge multiple PRs together) through the API in the future.
			refsToMerge := gitRefs.GetRefsToMerge()
			if refsToMerge != nil {
				if jobExecutionType == JobExecutionType_PRESUBMIT || jobExecutionType == JobExecutionType_POSTSUBMIT {
					if len(refsToMerge) > 1 {
						return fmt.Errorf("cannot have more than 1 refsToMerge for %q", jobExecutionType)
					}
				}

				for _, refToMerge := range refsToMerge {
					if err := refToMerge.Validate(); err != nil {
						return fmt.Errorf("invalid refsToMerge entry: %s", err)
					}
				}
			}
		}

		if jobExecutionType != JobExecutionType_PERIODIC {
			// Non-periodic jobs must have a BaseRepo (default repo to clone)
			// defined.
			if gitRefs == nil {
				return fmt.Errorf("gitRefs must be defined for %q", jobExecutionType)
			}
			if err := gitRefs.ValidateGitReferenceDynamic(); err != nil {
				return fmt.Errorf("gitRefs: failed to validate: %s", err)
			}
		}
	*/

	// Finally perform some additional checks on the requested PodSpecOptions.
	podSpecOptions := req.GetPodSpecOptions()
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

			if len(k) > 63 {
				return fmt.Errorf("invalid label: exceeds 63 characters: %q", k)
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

/* FIXME: Revive this for Refs.
func (grs *GitReferenceStatic) Validate() error {
	url := grs.GetUrl()
	if len(url) == 0 {
		return errors.New("url cannot be empty")
	}
	commit := grs.GetCommit()
	if len(commit) == 0 {
		return errors.New("commit cannot be empty")
	}
	ref := grs.GetRef()
	if len(ref) == 0 {
		return errors.New("ref cannot be empty")
	}

	// URL must start with a http or https protocol prefix.
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("url does not start with http[s]://: %q", url)
	}

	// Commit SHA must be a 40-character hex string.
	var validSha = regexp.MustCompile(`^[0-9a-f]{40}$`)
	if !validSha.MatchString(commit) {
		return fmt.Errorf("invalid commit SHA: %q", commit)
	}

	// Git reference names have special restrictions, but we don't bother doing
	// the check here because it's too complicated:
	// https://git-scm.com/docs/git-check-ref-format.
	//
	// (Skip additional checks for `ref`.)

	return nil
}

func (grd *GitReferenceDynamic) ValidateGitReferenceDynamic() error {
	base := grd.GetBase()
	if base == nil {
		return errors.New("baseRepo: base repo cannot be nil")
	}
	if err := base.Validate(); err != nil {
		return fmt.Errorf("invalid base repo for baseRepo: %s", err)
	}

	refsToMerge := grd.GetRefsToMerge()
	for _, refToMerge := range refsToMerge {
		if err := refToMerge.Validate(); err != nil {
			return fmt.Errorf("invalid refsToMerge entry: %s", err)
		}
	}

	return nil
}

*/
