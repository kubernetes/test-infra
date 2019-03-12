# Overview
This code coverage tool calculates per file, per package and overall coverage on target directories. It generates the following artifacts
  - code coverage profile, which is produced by "[go test -coverprofile](https://blog.golang.org/cover)" and contains all block level code coverage data
  - XML file that stores file-level and package-level code coverage, formatted to be readable by TestGrid

The tool has two major modes of operation, based on whether it is running in post-submit or pre-submit workflow. 
Post-submit workflow runs on a specific commit on a branch and is typically triggered when commits are pushed to a branch (e.g. when a PR merges). 
Pre-submit workflow runs on the merge commit for a pull request and is typically triggered by PR creation or update.

The tool performs the following additional operations when running in pre-submit mode 
  - after running code coverage on target directories, it compares the new result with the one stored by 
  the post-submit workflow and generate coverage difference. 
  - it reports coverage changes (calculated above) to the pull request as a comment by a robot github account. 
  - it uses go tools to generate line by line coverage and stores the result in html, 
  with a link as part of the robot comment mentioned above.
  - it can be configured to return with a non-zero status if coverage falls below threshold.

Note that pre-submit mode can not be used without the post-submit mode, because post-submit results are consumed as a baseline by the pre-submit mode.

## Users
The pre-submit mode is intended for a developer to see the impact on code coverage of his/her commit. 

The post-submit mode, provides input for TestGrid - TestGrid is for repo managers and/or test infra team to monitor code coverage stats over time.

## Programming Language Supported
The code coverage tool only collect code coverage for Go files


# Design of Test Coverage Tool
The tool takes input from three sources
1. Target directory
  - It runs [test coverage profiling](https://blog.golang.org/cover) on target repository. 
    - target directory can be passed as flags when running the binary. E.g "--cov-target=./pkg/"
  - .gitattribute file
    - it uses git attribute to filter files (see latter section on workflows for details)
2. (In pre-submit workflow only) It reads previously stored post-submit code coverage profile from gcs bucket. The profile
serves as a base of comparison for determining the pull request's coverage delta.
3. Variables passed through flags. Here is a list of these variables.

    |flag       |meaning                            |sample value |
    | --------- | --------------------------------- | ----------- |
    |artifacts  |local directory to dump artifacts  |./artifacts  |
    |cov-target |target directories to run coverage |./pkg1 ./pkg2|
    |cov-threshold-percentage|coverage threshold in percentage|85 |
    |profile-name|file name for code coverage profile|coverage_profile.txt|
    |postsubmit-gcs-bucket|gcs bucket that stores code coverage profile in post-submit run|knative-prow|
    |postsubmit-job-name|job name in gcs bucket that stores code coverage profile in post-submit run|post-knative-serving-go-coverage|
    
Note that the last two flags are for pre-submit mode only. The last three flags are used to locate the code coverage profile in GCS bucket.

Here is the step-by-step description of the pre-submit and post-submit workflows

## Post-submit workflow
The tool produces & stores coverage profile for later presubmit jobs to compare against; in addition, it produces per-file and per-package coverage result as input for [TestGrid](https://github.com/kubernetes/test-infra/tree/master/testgrid). Testgrid can use the data produced here to display coverage trend in a tabular or graphical way. 

1. Generate coverage profile. Completion marker generated upon successful run. Both stored
 in artifacts directory.
    - Completion marker is used by later pre-submit job when searching for a healthy and complete 
    code coverage profile in the post-submit jobs
    - Successfully generated coverage profile may be used as the basis of comparison for coverage change, in pre-submit workflow
2. Read, filter, and summarizes data from coverage profile and store per-file coverage data
    - filter based on git attribute to ignore files with the following git attributes
      - linguist-generated
      - coverage-excluded
      
      An example of how these git attribute is used can be found [here](https://github.com/knative/serving/blob/master/.gitattributes)
    - Stores in the XML format, that is used by TestGrid, and dump it in artifacts directory
      - The XML should be a valid JUnit XML file. See 
  [JUnit XML format](https://www.ibm.com/support/knowledgecenter/en/SSQ2R2_14.1.0/com.ibm.rsar.analysis.codereview.cobol.doc/topics/cac_useresults_junit.html)
      - In addition to being a valid jUnit XML file, the file needs to have the properties specified [here](https://github.com/kubernetes/test-infra/tree/master/gubernator#gcs-layout) to be readable by TestGrid 
      - For each file that has a coverage level lower than the threshold, the corresponding entry in the XML should have a \<failure\> tag

## Pre-submit workflow
Runs code coverage tool to report coverage change from a PR

1. Generate coverage profile in artifacts directory
2. Read, filter, and summarizes data from coverage profile and store per-file coverage data (Same as the corresponding step in post-submit)
3. Calculate coverage changes. Compare the coverage file generated in this cycle against the most
 recent successful post-submit build. Coverage file for post-submit commits were generated in 
 post-submit workflow and stored in gcs bucket
4. Use PR data from github, git-attributes, as well as coverage change data calculated above, to 
produce a list of files that we care about in the line-by-line coverage report. produce line by 
line coverage html and add link to covbot report. Note that covbot is the robot github account 
used to report code coverage change results. See Covbot section for more details.
5. If any file in this commit has a coverage change, let covbot post presubmit coverage on github, under that conversation of the PR. 
  - The covbot comment should have the following information on each file with a coverage change
    - file name
    - old coverage (coverage before any change in the PR)
    - new coverage (coverage after applied all changes in the PR)
    - change the coverage
  - After each new posting, any previous posting by covbot should be removed

## Locally running presubmit and post-submit workflows
Both workflows may be triggered locally in command line, as long as all the required flags are 
supplied correctly. In addition, the following env var needs to be set:
- JOB_TYPE (one of 'presubmit', 'postsubmit', 'local-presubmit')

use 'local-presubmit' will run the presubmit workflow without posting result on github PR
  
## Covbot
As mentioned in the presubmit workflow section, covbot is the short name for the robot github 
account used to report code coverage change results. It can be created as a regular github 
account - it does not need to be named covbot as that name is already taken on Github. It only need a 
comment access to the repo it need to be run on. If the repo is private, it also need read access. 
  
After the robot account is created, download the github token and supply the path to the token 
file to code coverage binary, as the value for parameter "github-token"

# Usage with container based CI/CD system
We pack the test coverage feature in a container, that is triggered to run by a CI/CD system such as [prow](https://github.com/kubernetes/test-infra/tree/master/prow), in response to Github events such as pulls and merges.

Here is [an example of a dockerfile](https://github.com/kubernetes/test-infra/blob/a1e910ae6811a1821ad98fa28e6fad03972a8c20/coverage/Dockerfile) using [Docker](https://www.docker.com/). 
Here is [an example of a Makefile](https://github.com/kubernetes/test-infra/blob/a1e910ae6811a1821ad98fa28e6fad03972a8c20/coverage/Makefile) that builds and pushes the docker image on [Google Container Registry](https://cloud.google.com/container-registry/).

## Usage with prow
Prow is tested working well with this Code Coverage tool. It's usage is described below

- Prow can be used as the system to handle Github events mentioned in the two workflows. 
- Prow, in both workflows, supplies the flags and secrets for the binary, clones the repository, and uploads logs & artifacts to GCS bucket.

  - The pre-submit prow job is triggered by any new commit to a PR. At the end of the binary run, it can return a pass or fail status context to Github. [Tide](https://github.com/kubernetes/test-infra/tree/master/prow/cmd/tide) can use that status to block PR with low coverage.

  - The post-submit prow job is triggered by merge events to the base branch.

### Prow Job Specification
Here is an example of a pre-submit prow job spec that runs the coverage tool in a container (the container build file can be found [here](https://github.com/kubernetes/test-infra/blob/a1e910ae6811a1821ad98fa28e6fad03972a8c20/coverage/Makefile)). The args section includes all the arguments for the binary of the tool. 
```
  - name: pull-knative-serving-go-coverage
    labels:
      preset-service-account: "true"
    always_run: true
    optional: true
    decorate: true
    clone_uri: "git@github.com:knative/serving.git"
    spec:
      containers:
      - image: gcr.io/knative-tests/test-infra/coverage:latest
        imagePullPolicy: Always
        command:
        - "/coverage"
        args:
        - "--postsubmit-gcs-bucket=knative-prow"
        - "--postsubmit-job-name=post-knative-serving-go-coverage"
        - "--profile-name=coverage_profile.txt"
        - "--artifacts=$(ARTIFACTS)" # folder to store artifacts, such as the coverage profile
        - "--cov-target=./pkg/" # target directory of test coverage
        - "--cov-threshold-percentage=50" # minimum level of acceptable presubmit coverage on a per-file level
        - "--github-token=/etc/github-token/token"
      env:
      - name: GOOGLE_APPLICATION_CREDENTIALS
        value: /etc/service-account/service-account.json
      volumes:
      - name: github-token
        secret:
          secretName: covbot-token
      - name: service
        secret:
          secretName: service-account
      volumeMounts:
      - name: github-token
        mountPath: /etc/github-token
        readOnly: true
      - name: service
        mountPath: /etc/service-account
        readOnly: true
```
