# Status Context Migrator
The migratestatus tool is a maintenance utility used to safely switch a repo from one status context to another.

For example if there is a context named "CI tests" that needs to be moved by "CI tests v2" this tool can be used to copy every "CI tests" status into a "CI tests v2" status context and then mark every "CI tests" context as passing and retired. This ensures that no PRs are ever passing when they shouldn't be and doesn't block PRs that should be passing. The copy and retire phases can be run separately or together at once in move mode.

By default, this tool runs in dry-run mode and doesn't make any modifications.
Pass `--dry-run=false` to actually change statuses.

### Usage

###### Modes

This tool runs in one of three modes. Each mode is defined as a set of conditions on the statuses and an action to take if the conditions are met.

- **copy** mode conditions on PRs that have the context to copy, but not the destination context. For each of these PRs, the 'copy' context's status is copied to the destination context.
	- `$ ./migratestatus --copy="CI tests" --dest="CI tests v2" --tokenfile=$TOKEN_FILE --org=$ORG --repo=$REPO`
- **retire** mode can be used to retire an old context with or without replacement. By setting the context's state to "success" the retired context can be ignored.
	- With a replacement specified, the mode conditions on PRs that have the context to retire and the context that replaced the retired context. For each of these PRs, the context to retire has its state set to "success" and its description set to a message of the form "Context retired. Status moved to 'CI tests v2'.".
	- Without a replacement specified, the mode conditions on PRs that have the context to retire. For each of these PRs, the context has its state set to "success" and its description set to "Context retired without replacement."
	- `$ ./migratestatus --retire="CI tests" --dest="CI tests v2" --tokenfile=$TOKEN_FILE --org=$ORG --repo=$REPO`
- **move** mode conditions on PRs that have the context to move, but not the destination context. For each of these PRs, a copy and retire action are performed. In other words, the status of the context to move is copied to the 'dest' context and then retired with 'dest' as the replacement.
	- `$ ./migratestatus --move="CI tests" --dest="CI tests v2" --tokenfile=$TOKEN_FILE --org=$ORG --repo=$REPO`

###### Flags

The migratestatus binary is run locally and can be built by running `go build` from this directory. The binary accepts the following parameters:

```
Usage of ./migratestatus:
  -branch-filter string
    	A regular expression which the PR target branch must match to be modified. (Optional)
  -continue-on-error
    	Indicates that the migration should continue if context migration fails for an individual PR.
  -copy string
    	Indicates copy mode and specifies the context to copy.
  -description string
    	A URL to a page explaining why a context was migrated or retired. (Optional)
  -dest string
    	The destination context to copy or move to. For retire mode this is the context that replaced the retired context.
  -dry-run
    	Run in dry-run mode, performing no modifying actions. (default true)
  -github-endpoint value
    	GitHub's API endpoint (may differ for enterprise). (default https://api.github.com)
  -github-token-file string
    	DEPRECATED: use -github-token-path instead.  -github-token-file may be removed anytime after 2019-01-01.
  -github-token-path string
    	Path to the file containing the GitHub OAuth secret. (default "/etc/github/oauth")
  -move string
    	Indicates move mode and specifies the context to move.
  -org string
    	The organization that owns the repo.
  -repo string
    	The repo needing status migration.
  -retire string
    	Indicates retire mode and specifies the context to retire.
```

Run the binary with the `-h` flag to see the rest of the available flags.
