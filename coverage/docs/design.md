#Overview
This code coverage tool has two major features.
1. In post-submit workflow (when a pull request is merged), it runs code coverage and generates the 
  following artifacts in a GCS bucket
  - xml file that stores file-level and package-level code coverage, formatted to be readable by TestGrid
  - code coverage profile, which will be used later in presubmit workflow as a base for comparison
2. In pre-submit workflow (when a pull requested is created or updated with new commit)
  - it runs code coverage on target directories and compares the new result with the one stored by 
  the post-submit workflow and generate coverage difference. It reports coverage changes to the 
  pull request as a comment by a robot github account. 
  - it uses go tools to generate line by line coverage and serve the result in html, with a link as 
  part of the robot comment mentioned above 
  - it can report a failure status if coverage falls below threshold - this allows repo owner to make the 
check required and block PR if coverage threshold not met. 


##Users
The presubmit tool is intended for a developer to see the impact on code coverage of his/her commit. It can also be used by repo managers to block any PR from merging if the coverage falls under customized threshold.

The periodical testgrid report is for repo managers and/or test infra team to monitor code coverage stats over time.

##Programming Language Supported
The code coverage tool only collect code coverage for Go files


#Design of Test Coverage Tool
We pack the test coverage feature in a container, that is triggered to run by a CI/CD system such as prow, in response to Github events such as pulls and merges.The behavior varies in presubmit and postsubmit workflows, which is discussed in latter sections. 

The tool takes input from three sources
1. It runs test coverage profiling on target repository. 
2. It reads previously stored post-submit code coverage profile from gcs bucket. The profile
serves as a base of comparison for presubmit delta coverage.
3. Variables passed through flags. Those variables include directory to run test coverage, file filters and threshold for desired coverage level.  

Here is the step-by-step description of the pre-submit and post-submit workflows
##Pre-submit workflow
Runs code coverage tool to report coverage change in a new PR or updated PR
1. Developer submit new commit to an open PR on github
2. Matching pre-submit job is started 
3. Generate coverage profile in artifacts directory
4. Calculate coverage changes. Compare the coverage file generated in this cycle against the most
 recent successful post-submit build. Coverage file for post-submit commits were generated in 
 post-submit workflow and stored in gcs bucket
5. Use PR data from github, git-attributes, as well as coverage change data calculated above, to 
produce a list of files that we care about in the line-by-line coverage report. produce line by 
line coverage html and add link to covbot report. Note that covbot is the robot github account 
used to report code coverage change results. See Covbot section for more details.
6. Let covbot post presubmit coverage on github, under that conversation of the PR. 

##Post-submit workflow
Produces & stores coverage profile for later presubmit jobs to compare against; 
Produces periodical coverage result as input for TestGrid. Testgrid can use the data produced here to display coverage trend in a tabular or graphical way. 
1. A PR is merged
2. Post-submit job started
3. Generate coverage profile. Completion marker generated upon successful run. Both stored
 in artifacts directory.
    - Completion marker is used by later pre-submit job when searching for a healthy and complete 
    code coverage profile in the post-submit jobs
    - Successfully generated coverage profile may be used as the basis of comparison for coverage change, 
    as mentioned in pre-submit workflow
4. Generate / store per-file coverage data
    - Stores in the XML format, that is used by TestGrid, and dump it in artifacts directory

##Locally running presubmit and post-submit workflows
Both workflows may be triggered locally in command line, as long as all the required flags are 
supplied correctly. In addition, the following env var needs to be set:
- JOB_TYPE (one of 'presubmit', 'postsubmit', 'periodic', 'local-presubmit')

use 'local-presubmit' will run the presubmit workflow without posting result on github PR
  
##Covbot
As mentioned in the presubmit workflow section, covbot is the short name for the robot github 
account used to report code coverage change results. It can be created as a regular github 
account - it does not need to be named covbot as that name is already taken on Github. It only need a 
comment access to the repo it need to be run on. If the repo is private, it also need read access. 
  
After the robot account is created, download the github token and supply the path to the token 
file to code coverage binary, as the value for parameter "github-token"

#Usage with prow

Prow can be used as the system to handle Github events mentioned in the two workflows. We can add a pre-submit prow job that is triggered by any new commit to a PR to run test coverage on the new build to compare it with the master branch and previous commit for pre-submit coverage. We can add a post-submit prow job that is triggered by merge events, to run test coverage when ever there is a merge on the target branch. 

In addition, at the end of each workflow, prow copies the artifacts directory to gcs bucket


##Prow Configuration File
As mentioned earlier, we use configuration file to store repository specific information. Below is an example that contains the args that will be supplied to the coverage container
```
  - name: pull-knative-serving-go-coverage
    labels:
      preset-service-account: "true"
    always_run: true
    optional: true
    decorate: true
    clone_uri: "git@github.com:knative/serving.git"
    ssh_key_secrets:
    - ssh-knative
    spec:
      containers:
      - image: gcr.io/knative-tests/test-infra/coverage:latest
        imagePullPolicy: Always
        command:
        - "/coverage"
        args:
        - "--postsubmit-gcs-bucket=knative-prow"
        - "--postsubmit-job-name=post-knative-serving-go-coverage"
        - "--artifacts=$(ARTIFACTS)" # folder to store artifacts, such as the coverage profile
        - "--profile-name=coverage_profile.txt"
        - "--cov-target=./pkg/" # target directory of test coverage
        - "--cov-threshold-percentage=50" # minimum level of acceptable presubmit coverage on a per-file level
        - "--github-token=/etc/github-token/token"
        volumeMounts:
        - name: github-token
          mountPath: /etc/github-token
          readOnly: true
      volumes:
      - name: github-token
        secret:
          secretName: covbot-token
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
