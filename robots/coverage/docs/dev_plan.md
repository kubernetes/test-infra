# The development plan for open sourcing code coverage tool

The code coverage tool originally designed for Knative repos and is re-designed to be onboarded to Kubernetes repos. The code is broken up into several parts and the plan is here

1. Isolate the production of Junit XML (TestGrid’s input) from the coverage tool. Onboard that feature to Gopherage so that it can be run independently from the coverage tool. This part is completed in https://github.com/kubernetes/test-infra/pull/9887
   - further work: need Kubernete's team to make sure this tool is running as a job in post-submit and make sure TestGrid is configured to read from the right location
2. Create an isolated tool to calculate the difference of two coverage lists and produce a collection of individual coverage differences. The result is formatted optionally to a table in coverage bot post. This part is under review in https://github.com/kubernetes/test-infra/pull/10010. ETA Nov 27
3. Create an isolated tool to browse through a list of prow job build stored in GCS bucket and retrieve the coverage profile file stored in the latest successful build. ETA Dec 3rd
   - pre-req: We need to make sure post-submit job runs "go test -coverprofile" and produces the profile in artifacts. This part may need be completed by the Kubernetes team
4. Create a script to run the tool created in step 3 - the retrieved profile will serve as the base profile. Compare the new code coverage profile with the base profile, using the tool created in Step 2.
   - pre-req: before the script is run, the pre-submit job should have ran the "go test -coverprofile" and produced the new profile in artifacts. This part may need be completed by the Kubernetes team
5. Add a command to the above script to post the content (described in step 2) to corresponding Github PR. This part may need Kubernetes team's help to authorize a bot to do the posting
6. Need Kubernete's team to add the script to the post submit process
