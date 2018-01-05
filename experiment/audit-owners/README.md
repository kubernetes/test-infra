# audit-owners

This utility reviews the OWNERS files on a project and attempts to detect those
that are not collaborators.

## Usage

A valid GitHub Token needs to be set as an environment variable named `GITHUB_TOKEN`.

*Flags:*

* `-srcdir`: The location of the repo on the local filesystem to start inspecting
* `-org`: The name of the GitHub org to use when querying for collaborators
* `-repo`: The name of the GitHub repo to use when querying for collaborators

```
$ audit-owners -s ~/Code/k8s/charts -o kubernetes -r charts
INFO[0000] ListCollaborators(kubernetes, charts)         client=github
GitHub Logins not found as collaborators:
* jfelten
```