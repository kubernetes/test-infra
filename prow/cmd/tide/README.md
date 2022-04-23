# Tide

Tide is a [Prow](https://github.com/kubernetes/test-infra/blob/master/prow/README.md)
component for managing a pool of GitHub PRs that match a given set of criteria.
It will automatically retest PRs that meet the criteria ("tide comes in") and automatically merge
them when they have up-to-date passing test results ("tide goes out").

[Open Issues](https://github.com/kubernetes/test-infra/issues?utf8=%E2%9C%93&q=is%3Aopen+is%3Aissue+label%3Aarea%2Fprow%2Ftide)

## Documentation
- [I JUST WANT MY PR TO MERGE!](/prow/cmd/tide/pr-authors.md)
- [Configuring Tide](/prow/cmd/tide/config.md)
- [Maintainer's Guide](/prow/cmd/tide/maintainers.md)


## Features
- Automatically runs batch tests and merges multiple PRs together whenever possible.
- Ensures that PRs are tested against the most recent base branch commit before they are allowed to merge.
- Maintains a GitHub status context that indicates if each PR is in a pool or what requirements are missing.
- Supports blocking merge to individual branches or whole repos using specifically labelled GitHub issues.
- Exposes Prometheus metrics.
- Supports repos that have 'optional' status contexts that shouldn't be required for merge.
- Serves live data about current pools and a history of actions which can be consumed by [Deck](/prow/cmd/deck) to populate the [Tide dashboard](https://prow.k8s.io/tide), the [PR dashboard](https://prow.k8s.io/pr), and the [Tide history page](https://prow.k8s.io/tide-history).
- Scales efficiently so that a single instance with a single bot token can provide merge automation to dozens of orgs and repos with unique merge criteria. Every distinct 'org/repo:branch' combination defines a disjoint merge pool so that merges only affect other PRs in the same branch.
- Provides configurable merge modes ('merge', 'squash', or 'rebase').

## History

Tide was created in 2017 by @spxtr to replace `mungegithub`'s Submit Queue.  It was designed to manage a large number of repositories across organizations without using many API rate limit tokens by identifying mergeable PRs with GitHub search queries fulfilled by GitHub's v4 GraphQL API.

## Flowchart

```mermaid
    graph TD;
        subgraph github[GitHub]
            subgraph org/repo/branch
                head-ref[HEAD ref];
                pullrequest[Pull Request];
                status-context[Status Context];
            end
        end

        subgraph prow-cluster
            prowjobs[Prowjobs];
            config.yaml;
        end

        subgraph tide-workflow
            Tide;
            pools;
            divided-pools;
            pools-->|dividePool|divided-pools;
            filtered-pools;
            subgraph syncSubpool
                pool-i;
                pool-n;
                pool-n1;
                accumulated-batch-prowjobs-->|filter out <br> incorrect refs <br> no longer meet merge requirement|valid-batches;
                valid-batches-->accumulated-batch-success;
                valid-batches-->accumulated-batch-pending;
                
                status-context-->|fake prowjob from context|filtered-prowjobs;
                filtered-prowjobs-->|accumulate|map_context_best-result;
                map_context_best-result-->map_pr_overall-results;
                map_pr_overall-results-->accumulated-success;
                map_pr_overall-results-->accumulated-pending;
                map_pr_overall-results-->accumulated-stale;
                
                subgraph all-accumulated-pools
                    accumulated-batch-success;
                    accumulated-batch-pending;
                    accumulated-success;
                    accumulated-pending;
                    accumulated-stale;
                end

                accumulated-batch-success-..->accumulated-batch-success-exist{Exist};
                accumulated-batch-pending-..->accumulated-batch-pending-exist{Exist};
                accumulated-success-..->accumulated-success-exist{Exist};
                accumulated-pending-..->accumulated-pending-exist{Exist};
                accumulated-stale-..->accumulated-stale-exist{Exist};
                pool-i-..->require-presubmits{Require Presubmits};
                accumulated-batch-success-exist-->|yes|merge-batch[Merge batch];
                merge-batch-->|Merge Pullrequests|pullrequest;
                accumulated-batch-success-exist-->|no|accumulated-batch-pending-exist;
                accumulated-batch-pending-exist-->|no|accumulated-success-exist;
                accumulated-success-exist-->|yes|merge-single[Merge Single];
                merge-single-->|Merge Pullrequests|pullrequest;
                require-presubmits-->|no|wait;
                accumulated-success-exist-->|no|require-presubmits;
                require-presubmits-->|yes|accumulated-pending-exist;
                accumulated-pending-exist-->|no|can-trigger-batch{Can Trigger New Batch};
                can-trigger-batch-->|yes|trigger-batch[Trigger new batch];
                can-trigger-batch-->|no|accumulated-stale-exist;
                accumulated-stale-exist-->|yes|trigger-highest-pr[Trigger Jobs on Highest Priority PR];
                accumulated-stale-exist-->|no|wait;
            end
        end

        Tide-->pools[Pools - grouped PRs, prow jobs by org/repo/branch];
        pullrequest-->pools;
        divided-pools-->|filter out prs <br> failed prow jobs <br> pending non prow checks <br> merge conflict <br> invalid merge method|filtered-pools;
        head-ref-->divided-pools;
        prowjobs-->divided-pools;
        config.yaml-->divided-pools;
        filtered-pools-->pool-i;
        filtered-pools-->pool-n;
        filtered-pools-->pool-n1[pool ...];
        pool-i-->|report tide status|status-context;
        pool-i-->|accumulateBatch|accumulated-batch-prowjobs;
        pool-i-->|accumulateSerial|filtered-prowjobs;



        classDef plain fill:#ddd,stroke:#fff,stroke-width:4px,color:#000;
        classDef k8s fill:#326ce5,stroke:#fff,stroke-width:4px,color:#fff;
        classDef github fill:#fff,stroke:#bbb,stroke-width:2px,color:#326ce5;
        classDef pools-def fill:#00ffff,stroke:#bbb,stroke-width:2px,color:#326ce5;
        classDef decision fill:#ffff00,stroke:#bbb,stroke-width:2px,color:#326ce5;
        classDef outcome fill:#00cc66,stroke:#bbb,stroke-width:2px,color:#326ce5;
        class prowjobs,config.yaml k8s;
        class Tide plain;
        class status-context,head-ref,pullrequest github;
        class accumulated-batch-success,accumulated-batch-pending,accumulated-success,accumulated-pending,accumulated-stale pools-def;
        class accumulated-batch-success-exist,accumulated-batch-pending-exist,accumulated-success-exist,accumulated-pending-exist,accumulated-stale-exist,can-trigger-batch,require-presubmits decision;
        class trigger-highest-pr,trigger-batch,merge-single,merge-batch,wait outcome;
```
