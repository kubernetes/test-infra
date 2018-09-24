# Peribolos Documentation

Peribolos allows the org settings, teams and memberships to be declared in a yaml file. Github is then updated to match the declared configuration.

See the [kubernetes/org] repo, in particular the [merge] and [`update.sh`] parts of that repo for this tool in action.

### Etymology

A [peribolos] is a wall that encloses a court in Greek/Roman architecture.

## Org configuration

Extend the primary prow [`config.yaml`] document to include a top-level `orgs` key that looks like the following:

```yaml
orgs:
  this-org:
    # org settings
    company: foo
    email: foo
    name: foo
    description: foo
    has_organization_projects: true
    has_repository_projects: true
    default_repository_permission: read
    members_can_create_repositories: false

    # org member settings
    members:
    - anne
    - bob
    admins:
    - carl

    # team settings
    teams:
      node:
        # team config
        description: people working on node backend
        privacy: closed
        previously:
        - backend  # If a backend team exists, rename it to node

        # team members
        members:
        - anne
        maintainers:
        - jane
      another-team:
        ...
      ...
  that-org:
    ...
```

This config will:
* Ensure the org settings match the following:
  - Set the company, email, name and descriptions fields for the org to foo
  - Allow projects to be created at the org and repo levels
  - Give everyone read access to repos by default
  - Disallow members from creating repositories
* Ensure the following memberships exist:
  - anne and bob are members, carl is an admin
* Configure the node and another-team in the following manner:
  - Set node's description and privacy setting.
  - Rename the backend team to node
  - Add anne as a member and jane as a maintainer to node
  - Similar things for another-team (details elided)

Note that any fields missing from the config will not be managed by peribolos. So if description is missing from the org setting, the current value will remain.

For more details please see GitHub documentation around [edit org], [update org membership], [edit team], [update team membership].

## Settings

In order to mitigate the chance of applying erroneous configs, the peribolos binary includes a few safety checks:

* `--required-admins=` - a list of people who must be configured as admins in order to accept the config (defaults to empty list)
* `--min-admins=5` - the config must specify at least this many admins
* `--require-self=true` - require the bot applying the config to be an admin.

These flags are designed to ensure that any problems can be corrected by rerunning the tool with a fixed config and/or binary.

* `--maximimum-removal-delta=0.25` - reject a config that deletes more than 25% of the current memberships.

This flag is designed to protect against typos in the configuration which might cause massive, unwanted deletions. Raising this value to 1.0 will allow deleting everyone, and reducing it to 0.0 will prevent any deletions.

* `--confirm=false` - no github mutations will be made until this flag is true. It is safe to run the binary without this flag. It will print what it would do, without actually making any changes.


See `bazel run //prow/cmd/peribolos -- --help` for the full and current list of settings that can be configured with flags.



[`config.yaml`]: /prow/config.yaml
[edit team]: https://developer.github.com/v3/teams/#edit-team
[edit org]: https://developer.github.com/v3/orgs/#edit-an-organization
[peribolos]: https://en.wikipedia.org/wiki/Peribolos
[update org membership]: https://developer.github.com/v3/orgs/members/#add-or-update-organization-membership
[update team membership]: https://developer.github.com/v3/teams/members/#add-or-update-team-membership
[merge]: https://github.com/kubernetes/org/tree/master/cmd/merge
[kubernetes/org]: https://github.com/kubernetes/org
[`update.sh`]: https://github.com/kubernetes/org/blob/master/admin/update.sh
