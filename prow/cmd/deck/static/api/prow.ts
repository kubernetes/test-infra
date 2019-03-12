export type JobType = "presubmit" | "postsubmit" | "batch" | "periodic";
export type JobState = "triggered" | "pending" | "success" | "failure" | "aborted" | "error" | "unknown" | "";

export interface Job {
  type: JobType;
  repo: string;
  refs: string;
  base_ref: string;
  base_sha: string;
  pull_sha: string;
  number: number;
  author: string;
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
