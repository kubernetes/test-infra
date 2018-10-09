export type MergeableState = "MERGEABLE" | "CONFLICTING" | "UNKNOWN";
export type StatusState = "EXPECTED" | "ERROR" | "FAILURE" | "PENDING" | "SUCCESS";

export interface Ref {
  Name: string;
  Prefix: string;
}

export interface Author {
  Login: string;
}

export interface Context {
  Context: string;
  Description: string;
  State: StatusState;
}


export interface Commit {
  Status: {
    Contexts: Context[];
  }
  OID: string;
}

export interface Repository {
  Name: string;
  NameWithOwner: string;
  Owner: {
    Login: string;
  };
}

export interface Milestone {
  Title: string;
}

export interface PullRequest {
  Number: number;
  Author: Author;
  BaseRef: Ref;
  HeadRefOID: string;
  Mergeable: MergeableState;
  Repository: Repository;
}