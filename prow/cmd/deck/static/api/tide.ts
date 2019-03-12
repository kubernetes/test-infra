import {Commit, PullRequest as BasePullRequest} from "./github";

export interface TideQuery {
  orgs?: string[];
  repos?: string[];
  excludedRepos?: string[];
  excludedBranches?: string[];
  includedBranches?: string[];
  labels?: string[];
  missingLabels?: string[];
  milestone?: string;
  reviewApprovedRequired?: boolean;
}

export interface PullRequest extends BasePullRequest {
  Title: string;
  HeadRefName: string;
  Commits: {
    Nodes: {
      Commit: Commit;
    }[];
  };
  Labels: {
    Nodes: {
      Name: string;
    }[];
  };
  Milestone?: {
    Title: string;
  };
}

export type Action = "WAIT" | "TRIGGER" | "TRIGGER_BATCH" | "MERGE" | "MERGE_BATCH" | "BLOCKED";

export interface Blocker {
  Number: number;
  Title: string;
  URL: string;
}

export interface TidePool {
  Org: string;
  Repo: string;
  Branch: string;

  SuccessPRs: PullRequest[];
  PendingPRs: PullRequest[];
  MissingPRs: PullRequest[];

  BatchPending: PullRequest[];

  Action: Action;
  Target: PullRequest[];
  Blockers: Blocker[];
}

export interface TideData {
  Queries: string[];
  TideQueries: TideQuery[];
  Pools: TidePool[];
}
