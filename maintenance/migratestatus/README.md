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
- `--dry-run` Whether to perform modifying actions. Defaults to true.
- `--logtostderr` Print logging to stderr. Recommended if you want to see what's
  happening.
- `--tokenfile` The file containing the github authentication token to use.
- `--org` The organization that owns the repository to migrate contexts in. (Kubernetes)
- `--repo` The repository to migrate contexts in.
- Exactly one of the following flags must appear. Each indicates a mode and specifies a context:
	- `--copy` The context to copy.
	- `--retire` The context to retire.
	- `--move` The context to move (copy and retire).
- `--dest` The destination context. This is the context that is copied **to** and/or the context that replaces the retired context. This flag may only be omitted if retire mode is specified and the old context is being retired without a replacement.

Run the binary with the `-h` flag to see the rest of the available flags.
