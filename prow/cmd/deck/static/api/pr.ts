import {Context, PullRequest as BasePullRequest} from './github';

export interface Label {
  ID: object;
  Name: string;
}

export interface Ref {
  Name: string;
  Prefix: string;
}

export interface Author {
  Login: string;
}

export interface Repository {
  Name: string;
  NameWithOwner: string;
  Owner: {
    Login: string;
  };
}

export interface PullRequest extends BasePullRequest {
  Merged: boolean;
  Title: string;
  Author: Author;
  BaseRef: Ref;
  Repository: Repository;
  Labels: {
    Nodes: {
      Label: Label;
    }[];
  };
  Milestone: {
    Title: string;
  };
}

export interface PullRequestWithContext {
  Contexts: Context[];
  PullRequest: PullRequest;
}

export interface UserData {
  Login: boolean;
  PullRequestsWithContexts: PullRequestWithContext[];
}
