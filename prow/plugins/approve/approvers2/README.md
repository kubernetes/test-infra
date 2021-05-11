# Granular Approval Support

The `granular_approval` support for the `approve` plugin can be used to alter the behavior of the `approve` plugin.

## Summary

If `granular_approval` is set to `true` the `approve` plugin allows approvers to granularly approve a PR , i.e, approve individual files instead of full directories.  
An approver can approve changes in the PR by using the `/approve` command in a comment, or retract it by typing `/approve cancel` (at the beginning of the comment line.)  
An approve can approve individual changes or a set of changes by using the `/approve files <path-to-files>` command in a comment.  

## Approvers

Approvers are people who have contributed substantially to the repo and can provide approval to changes to the repo. Approvers are defined in a file named "OWNERS" that is present in each GitHub directory which is a unit of independent code.  

The OWNERS file can be defined in one of the following ways:  

With an OWNERS file defined like below the user `ykakarap` and `nikhita` can approve the changes to any files in the directory (and sub-directories) that contains the OWNERS file.
```yaml
approvers:
- ykakarap
- nikhita
```

With an OWNERS file defined like below the user `ykakarap` can only approve files whose paths end in `_test.go` while the user `nikhita` can approve all the files in the directory (and sub-directories) that contains the OWNERS file.
```yaml
filters:
  ".*_test\\.go":
    approvers:
      - ykakarap
  
  ".*":
    approvers:
      - nikhita
```

Note that items in the OWNERS files can be GitHub usernames, or aliases defined in OWNERS_ALIASES files. An OWNERS_ALIASES file is another co-existed file that delivers a mechanism for defining groups. However, GitHub Team names are not supported. We do not use them because there is no audit log for changes to the GitHub Teams. This way we have an audit log.

## Design

A PR cannot be merged into the repo without the **approved** label. In order for the approved label to be applied, every file modified by the PR must be approved by an approver in the OWNERs files.  

An approver can approve changes to a single file, a collection of files or a directory using the following commands:
- `/approve files <path-to-a-single-file>`  
	approves the file in the given path.
- `/approve files <path-with-wild-card>`  
	approves all the files in the PR that match the wild card path.
- `/approve files <path-to-directory>/*`  
	approves all the files in the given directory.
- `/approve`  
	approves all the files in the PR the approver has permission to approve.

The process is best illustrated in the example below:

### Example
In a repo with the following folder structure:
```
pkg/
├── api/
|   ├── OWNERS  
└── registry/
    ├── OWNERS
	├── apps/
```
The OWNERS file at `pkg/api/OWNERS` is:
```yaml
filters: 
  .*: 
    approvers: 
      - nikhita
      - bob
  .*_test\.go: 
    approvers: 
      - ykakarap
```

The OWNERs file at `pkg/registry/OWNERS` is:
```yaml
approvers:
- ykakarap
- nikhita
- bob
```

Consider a PR which changes the following files:
```
pkg
├── api
│   ├── first.go
│   ├── first_test.go
│   ├── second.go
│   └── second_test.go
└── registry
    ├── apps
    │   ├── one.go
    │   └── one_test.go
    ├── first.go
    ├── first_test.go
    ├── second.go
    └── second_test.go
```

#### Step 1:
The k8s-bot creates a comment on the PR showing the initial status. The k8s-bot also suggests the selected approvers along with the list of OWNERS file where the approved can be found. More details on how approvers are selected is described below.

	[APPROVALNOTIFIER] This PR is NOT APPROVED

	This pull-request has been approved by: *PRAuthor*
	To complete the pull request process, please assign nikhita, ykakarap
	You can assign the PR to them by writing /assign @nikhita @ykakarap in a comment when ready.

	The full list of commands accepted by this bot can be found here.

	Out of 10 files: 0 are approved and 10 are unapproved.

	Needs approval from approvers in these files:

	* pkg/api/OWNERS
	* pkg/registry/OWNERS

	Approvers can indicate their approval by writing /approve in a comment
	Approvers can also choose to approve only specific files by writing /approve files <path-to-file> in a comment
	Approvers can cancel approval by writing /approve cancel in a comment
	The status of the PR is:

	* pkg/api/
	* pkg/registry/

The selected approver such as *ykakarap* can be notified by typing `/assign @ykakarap` in a comment.

#### Step 2:

*ykakarap* approves `pkg/api/first_test.go` by writing `/approve files pkg/api/first_test.go`.

K8s-bot updates comment:

	[APPROVALNOTIFIER] This PR is NOT APPROVED

	This pull-request has been approved by: *PRAuthor*, *ykakarap*
	To complete the pull request process, please assign nikhita
	You can assign the PR to them by writing `/assign @nikhita` in a comment when ready.

	The full list of commands accepted by this bot can be found here.

	Out of 10 files: 1 are approved and 9 are unapproved.

	Needs approval from approvers in these files:

	* pkg/api/OWNERS
	* pkg/registry/OWNERS
	
	Approvers can indicate their approval by writing /approve in a comment
	Approvers can also choose to approve only specific files by writing /approve files <path-to-file> in a comment
	Approvers can cancel approval by writing /approve cancel in a comment
	The status of the PR is:

	* pkg/api/ (partially approved, need additional approvals) [ykakarap]
	* pkg/registry/

Note that even though *ykakarap* can approve more files in the PR only 1 files in the PR was approved. The directry is `pkg/api/` is parritally approved and the PR status shows that only 1 files is approved and the remaining 9 files still need to be approved. 

#### Step 3:

*nikhita* approves all the files under `pkg/register/apps/` by writing `/approve files pkg/register/apps/*`.

K8s-bot updates comment:

	[APPROVALNOTIFIER] This PR is NOT APPROVED

	This pull-request has been approved by: *PRAuthor*, *ykakarap*, *nikhita*
	To complete the pull request process, please assign bob
	You can assign the PR to them by writing `/assign @bob` in a comment when ready.

	The full list of commands accepted by this bot can be found here.

	Out of 10 files: 3 are approved and 7 are unapproved.

	Needs approval from approvers in these files:

	* pkg/api/OWNERS
	* pkg/registry/OWNERS
	
	Approvers can indicate their approval by writing /approve in a comment
	Approvers can also choose to approve only specific files by writing /approve files <path-to-file> in a comment
	Approvers can cancel approval by writing /approve cancel in a comment
	The status of the PR is:

	* pkg/api/ (partially approved, need additional approvals) [ykakarap]
	* pkg/registry/ (partially approved, need additional approvals) [nikhita]

The 2 files (`pkg/registry/apps/one.go` and `pkg/registry/apps/one_test.go`) match the wild card pattern used in the approval comment and are approved.

#### Step 5:
*ykakara* approves all the remaining files in the `pkg/registry/` directry by writing `/approve files pkg/registry/*`.

K8s-bot updates comment:

	[APPROVALNOTIFIER] This PR is NOT APPROVED

	This pull-request has been approved by: *PRAuthor*, *ykakarap*, *nikhita*
	To complete the pull request process, please assign bob
	You can assign the PR to them by writing `/assign @bob` in a comment when ready.

	The full list of commands accepted by this bot can be found here.

	Out of 10 files: 7 are approved and 3 are unapproved.

	Needs approval from approvers in these files:

	* pkg/api/OWNERS
	
	Approvers can indicate their approval by writing /approve in a comment
	Approvers can also choose to approve only specific files by writing /approve files <path-to-file> in a comment
	Approvers can cancel approval by writing /approve cancel in a comment
	The status of the PR is:

	* pkg/api/ (partially approved, need additional approvals) [ykakarap]
	* ~pkg/registry/~ (approved) [nikhita, ykakarap]

The directory `pkg/registry/` is now completely approved. The PR only needs approval from approvers in the `pkg/api/OWNERS` file.

#### Step 6:

*nikhita* approves all the remaining changes by writing `/approve`.

K8s-bot updates comment:

	[APPROVALNOTIFIER] This PR is NOT APPROVED

	This pull-request has been approved by: *PRAuthor*, *ykakarap*, *nikhita*
	To complete the pull request process, please assign bob
	You can assign the PR to them by writing `/assign @bob` in a comment when ready.

	The full list of commands accepted by this bot can be found here.

	Out of 10 files: 10 are approved and 0 are unapproved.

	Approvers can indicate their approval by writing /approve in a comment
	Approvers can also choose to approve only specific files by writing /approve files <path-to-file> in a comment
	Approvers can cancel approval by writing /approve cancel in a comment
	The status of the PR is:

	* ~pkg/api/~ (approved) [ykakarap, nikhita]
	* ~pkg/registry/~ (approved) [nikhita, ykakarap]

The PR is now completely approved and the the bot assigs the label `approved` to the PR. If the PR also has the label `lgtm` the PR is then merged.

### Canceling an approval

At any point before the PR is merged the approver can revoke his/her approval by writing `/approve cancel` in a comment. This will revoke the approval given all the files till that point. 


### Suggested Approvers Mechanism

First, it is important to understand that ALL approvers in an OWNERS file can approve any file (according to the filters defined) in that directory AND its subdirectories.  

The suggested approvers selection algorithm is roughly:
* Construct the subset of approvers from the leaf OWNERS files (according to the files in the PR) without the approvers who already provided an approval or are already assigned.
* Construct the minimum set of approvers from this subset who can approve the remaining files in the PR
* Repeat the process with root approvers excluding current approvers, current assignees and leaf suggested approvers.
* Return the final set of suggested approvers

The exact algorithms for selecting approvers is somewhat complex; it is an set cover approximations with considerations for existing assignees and current approvers. To read it in more depth, check out the approvers source code linked at the end of the README.


