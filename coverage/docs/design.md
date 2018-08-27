#Overview
Code coverage tool has two major features.
1. As a pre-submit tool, it runs code coverage on every single commit to Github and reports coverage change back to the PR as a comment by a robot account. It also has the ability to block a PR from merging if coverage falls below threshold
2. As a post-submit / periodical running job, it reports on TestGrid to show users how coverage changes over time.

##Background - Prow
Prow is a system that handles github events and commands and allow you to perform actions. It was originally built for kubernetes, but now is extended to other teams wanting to use it as well. cmd/hook is the main entry point for Prow and listens to the github events. Prow provides two ways to handle events:
1. Prow Jobs: Jobs that perform simple actions when certain events occur. Can only report the result as PASS or FAIL. For eg. running tests
2. Plugins: Some logic that perform more complicated actions like talking to external service. Can report any kind of status. For eg. golint plugin that checks out code from github and performs linting. Plugins can be:
  - Internal: Live within the cmd/hook binary.
  - External: Live as a separate binary. Events are forwarded to these by cmd/hook.

#Design of Test Coverage Tool
We pack the test coverage feature in a container, that is triggered by prow. The feature takes input from three sources
1. It runs test coverage profiling on target repository. Prow clones the target repository as the current working directory for the container.
2. It receives github related variables, such as pull request number & commit number, from Prow. Those variables are used as meta-data for the profiles. Metadata allows us to do presumbit coverage comparisons.
3. It allows user to supply variables in prowjob configs. Those variables include directory to run test coverage, file filters and threshold for desired coverage level.  

Prow has handlers for different github events. We add a pre-submit prow job that is triggered by any new commit to a PR to run test coverage on the new build to compare it with the master branch and previous commit for pre-submit coverage. We add a post-submit prow job that is triggered by merge events, to run test coverage on the nodes of master branch. Test coverage data on the master branch is supplied to TestGrid for displaying the coverage change over time, as well as serve as the basis of comparison for pre-submit coverage mentioned in the pre-submit senario.

Here is the step-by-step description of the pre-submit and post-submit workflows

##Pre-submit workflow
Runs code coverage tool to report coverage change in a new commit or new PR
1. Developer submit new commit to an open PR on github
2. Matching pre-submit prow job is started 
3. Test coverage profile generated
4. Calculate coverage change against master branch. Compare the coverage file generated in this cycle against the most recent successful post-submit build. Coverage file for post-submit commits were generated in post-submit workflow
5. Use PR data from github, git-attributes, as well as coverage change data calculated above, to produce a list of files that we care about in the line-by-line coverage report. produce line by line coverage html and add link to covbot report.
6. Let covbot post presubmit coverage on github, under that conversation of the PR. 

##Post-submit workflow
Produces & stores coverage profile for later presubmit jobs to compare against
1. A PR is merged
2. Post-submit prow job started
3. Test coverage profile generated. Completion marker generated upon successful run.

##Periodical workflow
Produces periodical coverage result as input for TestGrid
1. Periodical prow job starts periodically based on the specification in prow job config
2. Test coverage profile & metadata generated 
3. Generate / store per-file coverage data
  - Stores in the XML format that is used by TestGrid
  
##Prow Configuration File
As mentioned earlier, we use configuration file to store repository specific information. Below is an example that contains the args that will be supplied to the coverage container
```
    spec:
      containers:
      - image:  gcr.io/coverage-prow/coverage:version123
        args:
        - "--artifacts=./artifacts/" # folder to store artifacts, such as the coverage profile
        - "--target=./pkg/" # target directory of test coverage
        - "--coverage_threshold=70%" # minimum level of acceptable presubmit coverage on a per-file level
```


 

