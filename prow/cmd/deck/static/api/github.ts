export type MergeableState = "MERGEABLE" | "CONFLICTING" | "UNKNOWN";
export type StatusState = "EXPECTED" | "ERROR" | "FAILURE" | "PENDING" | "SUCCESS";

export interface Context {
  Context: string;
  Description: string;
  State: StatusState;
}

export interface Commit {
  Status: {
    Contexts: Context[];
  };
  OID: string;
}

export interface Milestone {
  Title: string;
}

export interface PullRequest {
  Number: number;
  HeadRefOID: string;
  Mergeable: MergeableState;
}
