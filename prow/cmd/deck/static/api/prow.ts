export type ProwJobType = "presubmit" | "postsubmit" | "batch" | "periodic";
export type ProwJobState = "triggered" | "pending" | "success" | "failure" | "aborted" | "error" | "unknown" | "";
export type ProwJobAgent = "kubernetes" | "jenkins" | "tekton-pipeline";

// Pull describes a pull request at a particular point in time.
// Pull mirrors the Pull struct defined in prow/apis/prowjobs/v1/types.go.
export interface Pull {
  number: number;
  author: string;
  sha: string;
  title?: string;
  ref?: string;
  link?: string;
  commit_link?: string;
  author_link?: string;
}

// Refs describes how the repo was constructed.
// Refs mirrors the Refs struct defined in prow/apis/prowjobs/v1/types.go.
export interface Refs {
  org: string;
  repo: string;
  repo_link?: string;
  base_ref?: string;
  base_sha?: string;
  base_link?: string;
  pulls?: Pull[];
  path_alias?: string;
  clone_uri?: string;
  skip_submodules?: boolean;
}

// ProwJobList is a list of ProwJob resources.
// ProwJobList mirrors the ProwJobList struct defined in prow/apis/prowjobs/v1/types.go.
export interface ProwJobList {
  kind?: string;
  apiVersion?: string;
  metadata: ListMeta;
  items: ProwJob[];
}

// ProwJob contains the spec as well as runtime metadata.
// ProwJob mirrors the ProwJob struct defined in prow/apis/prowjobs/v1/types.go.
export interface ProwJob {
  kind?: string;
  apiVersion?: string;
  metadata: ObjectMeta;
  spec: ProwJobSpec;
  status: ProwJobStatus;
}

// ListMeta describes metadata that synthetic resources must have, including lists and
// various status objects. A resource may have only one of {ObjectMeta, ListMeta}.
// ListMeta mirrors the ListMeta struct defined in k8s.io/apimachinery/pkg/apis/meta/v1/types.go.
export interface ListMeta {
  selfLink?: string;
  resourceVersion?: string;
  continue?: string;
}

// ObjectMeta is metadata that all persisted resources must have, which includes all objects
// users must create.
// ObjectMeta mirrors the ObjectMeta struct defined in k8s.io/apimachinery/pkg/apis/meta/v1/types.go.
export interface ObjectMeta {
  name?: string;
  generateName?: string;
  namespace?: string;
  selfLink?: string;
  uid?: string;
  resourceVersion?: string;
  generation?: number;
  creationTimestamp: string;
  deletionTimestamp?: string;
  deletionGracePeriodSeconds?: number;
  labels?: { [key: string]: string };
  annotations?: { [key: string]: string };
  ownerReferences?: object[];
  initializers?: object;
  finalizers?: string[];
  clusterName?: string;
  managedFields?: object[];
}

// ProwJobSpec configures the details of the prow job.
//
// Details include the podspec, code to clone, the cluster it runs
// any child jobs, concurrency limitations, etc.
// ProwJobSpec mirrors the ProwJobSpec struct defined in prow/apis/prowjobs/v1/types.go.
export interface ProwJobSpec {
  type?: ProwJobType;
  agent?: ProwJobAgent;
  cluster?: string;
  namespace?: string;
  job?: string;
  refs?: Refs;
  extra_refs?: Refs;
  report?: boolean;
  context?: string;
  rerun_command?: string;
  max_concurrency?: number;
  error_on_eviction?: boolean;
  pod_spec?: PodSpec;
  build_spec?: object;
  jenkins_spec?: object;
  pipeline_run_spec?: object;
  decoration_config?: object;
  reporter_config?: object;
  rerun_auth_config?: object;
  hidden?: boolean;
}

// ProwJobStatus provides runtime metadata, such as when it finished, whether it is running, etc.
// ProwJobStatus mirrors the ProwJobStatus struct defined in prow/apis/prowjobs/v1/types.go.
export interface ProwJobStatus {
  startTime: string;
  completionTime?: string;
  state?: ProwJobState;
  description?: string;
  url?: string;
  pod_name?: string;
  build_id?: string;
  jenkins_build_id?: string;
  prev_report_states?: { [key: string]: ProwJobState };
}

// PodSpec is a description of a pod.
// PodSpec mirrors the PodSpec struct defined in k8s.io/api/core/v1
// Podspec interface only holds containers right now since no other values are used
export interface PodSpec {
  containers: object[];
}
