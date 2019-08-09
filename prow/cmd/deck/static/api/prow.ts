export type JobType = "presubmit" | "postsubmit" | "batch" | "periodic";
export type JobState = "triggered" | "pending" | "success" | "failure" | "aborted" | "error" | "unknown" | "";

// Pull describes a pull request at a particular point in time.
// Pull mirrors the Pull struct defined in types.go.
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
// Refs mirrors the Refs struct defined in types.go.
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

export interface Job {
  type: JobType;
  refs: Refs;
  refs_key: string;
  job: string;
  build_id: string;
  context: string;
  started: string;
  finished: string;
  duration: string;
  state: JobState;
  description: string;
  url: string;
  pod_name: string;
  agent: string;
  prow_job: string;
}
