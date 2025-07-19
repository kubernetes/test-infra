The project has certain guidelines around jobs which are meant to ensure that
there's a balance between test coverage and costs for running the CI. For
example, non-blocking jobs that get trigger automatically for PRs should be
used judiciously.

Because SIG leads are not necessarily familiar with those policies, SIG Testing
and SIG Infra need to be involved before merging jobs that fall into those
sensitive areas. This is achieved with tests and additional files in this
directory and a separate OWNERS file.

To check whether jobs are okay, run the Go tests in this directory.
If tests fail, re-run with the `UPDATE_FIXTURE_DATA=true` env variable
and include the modified files in the PR which updates the jobs.
