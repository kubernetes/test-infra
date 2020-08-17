# Playbook Index

This is an index of the oncall playbooks for our various services.

These are intended to help you diagnose and repair our infrastructure.

<!--TODO: add short entries for each service we host-->

## Prow

[Playbook][prow-playbook]

TDLR: Prow is a set of CI services that we run.

In particular we use this for hosting Kubernetes's CI and GitHub automation.

## Greenhouse

[Playbook][greenhouse-playbook]

TDLR: Greenhouse is a bazel [remote build cache] service.

In particular we use this for building the [Kubernetes repo][kubernetes-repo] 
in presubmit on Prow.

<!--URLS-->
[kubernetes-repo]: https://github.com/kubernetes/kubernetes
[greenhouse-playbook]: ./../greenhouse/playbook.md
[prow-playbook]: ./../prow/playbook.md
[remote build cache]: https://docs.bazel.build/versions/master/remote-caching.html
