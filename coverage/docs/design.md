#Overview
This code coverage tool has two major features.
1. As a pre-submit tool, it runs code coverage on every single commit to Github and reports coverage change back to the PR as a comment by a robot account. It also has the ability to block a PR from merging if coverage falls below threshold
2. As a post-submit / periodical running job, it reports on TestGrid to show users how coverage changes over time.

##Users
The presubmit tool is intended for a developer to see the impact on code coverage of his/her commit. It can also be used by repo managers to block any PR from merging if the coverage falls under customized threshold.

The periodical testgrid report is for repo managers and/or test infra team to monitor code coverage stats over time.

##Limitation
As of now, the code coverage tool only collect code coverage for Go files. The support for more programming languages may be added later

##Background - Prow
Prow is a system that handles github events and commands and allow you to perform actions. It was originally built for kubernetes, but now is extended to other teams wanting to use it as well. cmd/hook is the main entry point for Prow and listens to the github events. Prow provides two ways to handle events:
1. Prow Jobs: Jobs that perform simple actions when certain events occur. Can only report the result as PASS or FAIL. For eg. running tests
2. Plugins: Some logic that perform more complicated actions like talking to external service. Can report any kind of status. For eg. Golint plugin that checks out code from github and performs linting. Plugins can be:
  - Internal: Live within the cmd/hook binary.
  - External: Live as a separate binary. Events are forwarded to these by cmd/hook.

#Design of Test Coverage Tool
We pack the test coverage feature in a container, that is triggered by prow as a prow job. There is a separate prow job configured for each of the following workflows (which is discussed in later sections): pre-submit, post-submit and periodic. 

The feature takes input from three sources
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
4. Calculate coverage changes. Compare the coverage file generated in this cycle against the most recent successful post-submit build. Coverage file for post-submit commits were generated in post-submit workflow
5. Use PR data from github, git-attributes, as well as coverage change data calculated above, to produce a list of files that we care about in the line-by-line coverage report. produce line by line coverage html and add link to covbot report. Note that covbot is the robot github account used to report code coverage change results.
6. Let covbot post presubmit coverage on github, under that conversation of the PR. 
7. When coverage threshold is enforced, block PR from merging by making this prow job 'required' and return with a code other than 0

##Post-submit workflow
Produces & stores coverage profile for later presubmit jobs to compare against
1. A PR is merged
2. Post-submit prow job started
3. Test coverage profile generated. Completion marker generated upon successful run. Both stored as prow artifacts.
    - Completion marker is used by later pre-submit job when searching for a healthy and complete code coverage profile in the post-submit jobs

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

#Acceptance Criteria
##Presubmit Workflow
- When user made a new commit, if any file in the commit has a coverage change, post a covbot comment concluding the coverage change
- When user made a new commit, if non of the files in the commit has a coverage change, do not post any comment
- The covbot comment should have the following information on each file with a coverage change:
  - file name
  - old coverage (coverage before any change in the PR)
  - new coverage (coverage after applied all changes in the PR)
  - change the coverage
- After each new posting, any previous posting by covbot should be removed
- When coverage threshold is enforced, block PR from merging

##Periodical Workflow
- During each run of the periodical job, a junit_bazel.xml is dumped in the artifacts folder
- The junit_bazel.xml should be a valid junit xml file. See 
[JUnit XML format](https://www.ibm.com/support/knowledgecenter/en/SSQ2R2_14.1.0/com.ibm.rsar.analysis.codereview.cobol.doc/topics/cac_useresults_junit.html)
- For each file that has a coverage level lower than the threshold, the corresponding entry on testgrid should be red; otherwise, the entry should be green
