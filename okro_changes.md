# Okro changes

This is a list of all the changes made to this repository to support Okro use cases:

- Prow plugins (traiana/prow/plugins):
  - okro/undoer ([commit](https://github.com/traiana/test-infra/commit/42d85973f83e4f39cd592543be144cef7a4f6d09))

- Allow setting the logger for github and git clients ([commit](https://github.com/traiana/test-infra/commit/1666ebeb81d6b7ee303c642f0b9f38e98bca5d51))

- Add EditIssue method to github client ([commit](https://github.com/traiana/test-infra/commit/1e4254e8faa45936bfb4090430f10ac3de21d437 ))

- Fix missing prowjob URL bug, due to race between crier and build controller ([commit](https://github.com/traiana/test-infra/commit/d6129dd927ce985baa0f0bbd8ece3fbf1cd5a505))

- Temp patch for context bug in build controller. Two informers (one for context "" and one for context "default") are competing
over builds and creating/deleting them infinitely. Somehow related to [kubernetes#11029](https://github.com/kubernetes/test-infra/issues/11029) ([commit](https://github.com/traiana/test-infra/commit/fb2a1b7c9decf013cf8cfa19c8744e3a5c6f19ab))

- Comment out Deck code that handles gcs symlinks and makes spyglass not work with s3 ([commit](https://github.com/traiana/test-infra/commit/3992cb26b37b05b50bee9417c04c603d0d4d8357))

- Support nice JSON logging in spyglass build logs ([commit](https://github.com/traiana/test-infra/commit/baf2edc76450eece5cf60253b899f442c7d2bfd9))
  - make build logs in Spyglass respect whitespace so we can print JSON responses from our API in human-readable format.

- Reduce verbosity of failed build jobs statuses ([commit](https://github.com/traiana/test-infra/commit/6b1c7d0613ff48c3182e3e284778f83221f0f012))
  - make Github status description for knative-build prowjobs be similar to kubernetes prowjobs (e.g "Job failed" instead of "step X exited with status 1")

- Allow skipping build jobs steps ([commit](https://github.com/traiana/test-infra/commit/5b8af55c4fa160a26cbf558c7b9001b2531ac99e))
  - Tide may run presubmit jobs multiple times before it merges a PR. The validate_build step during our build process
    used to fail if the build existed, which would fail the whole presubmit job if ran more than once. It now checks
    whether the build is exactly the same as the existing one and if it is - mark the step as passed and skip all other
    steps (pushing images, registering the build, etc).

- Allow authors to lgtm their own PRs when they're the only reviewers ([commit](https://github.com/traiana/test-infra/commit/f2077f6c5fc22de520af5dd86c47725fc4ed7334))
  - to allow users to merge changes to their own realms.

- Change `/test` and `/retest` commands to `/run` and `/rerun` ([commit](https://github.com/traiana/test-infra/commit/262ef6edb8b8c684b164b776ad34c63223c928a3))
  - because they are also used to run validate jobs and builds.
  
- Add short sha env variables to decorated pods ([commit](https://github.com/traiana/test-infra/commit/2a9a0e38373a9d0d71934eb5177a9b4e17793e32))
  - add the PULL_BASE_SHA_SHORT and PULL_PULL_SHA_SHORT env variables which contain the short version (7 characters) of
    PULL_BASE_SHA and PULL_PULL_SHA respectively. Used for interpolation when creating image names.

- Add CreateIssue method to github client ([commit](https://github.com/traiana/test-infra/commit/c4eb98e3974952579becad07fd7b9c6ebd05c181))

- Add download/upload support for AWS ([commit](https://github.com/traiana/test-infra/commit/399221e950277f1a3fd58408628e207bf96d6ca2))

- Exapnd BUILD_VERSION environment variable in entrypoint ([commit](https://github.com/traiana/test-infra/commit/dcff9a87d60d9ea8a02f4111e075b92714c6ddaf))

