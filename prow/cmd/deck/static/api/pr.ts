import {Context, PullRequest as BasePullRequest} from './github';

export interface Label {
  ID: object;
  Name: string;
}

export interface PullRequest extends BasePullRequest {
  Merged: boolean;
  Title: string;
  Labels: {
    Nodes: {
      Label: Label;
    }[];
  };
  Milestone: {
    Title: string;
  }
}

export interface PullRequestWithContext {
  Contexts: Context[];
  PullRequest: PullRequest;
}

export interface UserData {
  Login: boolean;
  PullRequestsWithContexts: PullRequestWithContext[];
}
