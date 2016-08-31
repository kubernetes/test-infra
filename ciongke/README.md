Here's how this works:

1. We run a small GKE cluster in the same project as PR Jenkins.
1. On this cluster, we run a deployment/service that listens for GitHub
   webhooks (`cmd/hook`). When an event of interest comes in, such as a new PR
   or an "ok to test" comment, we check that it's safe to test, and then start
   up the corresponding jobs.
1. The jobs themselves (`cmd/test-pr`) start and watch the Jenkins job, setting
   the GitHub status along the way.
