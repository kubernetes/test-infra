# Not implemented

WARNING: none of this code works yet. Implementation will follow later

# Peribolos Documentation

Peribolos allows the org settings, teams and memberships to be declared in a yaml file. Github is then updated to match the declared configuration.

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


[`config.yaml`]: /prow/config.yaml
[edit team]: https://developer.github.com/v3/teams/#edit-team
[edit org]: https://developer.github.com/v3/orgs/#edit-an-organization
[peribolos]: https://en.wikipedia.org/wiki/Peribolos
[update org membership]: https://developer.github.com/v3/orgs/members/#add-or-update-organization-membership
[update team membership]: https://developer.github.com/v3/teams/members/#add-or-update-team-membership
