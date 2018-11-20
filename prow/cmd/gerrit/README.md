# Gerrit

Gerrit is a Prow-gerrit adapter for handling CI on gerrit workflows. It can poll gerrit
changes from multiple gerrit instances, and trigger presubmits on Prow upon new patchsets
on Gerrit changes, and postsubmits when Gerrit changes are merged.

## Deployment Usage

When deploy the gerrit component, you need to specify `--config-path` to your prow config, and optionally
`--job-config-path` to your prowjob config if you have split them up.

Set `--gerrit-projects` to the gerrit projects you want to poll against.

Example:
If you want prow to interact with gerrit project `foo` and `bar` on instance `gerrit-1.googlesource.com`
and also project `baz` on instance `gerrit-2.googlesource.com`, then you can set:

```
--gerrit-projects=gerrit-1.googlesource.com=foo,bar
--gerrit-projects=gerrit-2.googlesource.com=baz
```

`--cookiefile` allows you to specify a git https cookie file to interact with your gerrit instances, leave
it empty for anonymous access to gerrit API.

`--last-sync-fallback` should point to a persistent volume that saves your last poll to gerrit.

## Underlying infra

Also take a look at [gerrit related packages](/prow/gerrit/README.md) for implementation details.

You might also want to deploy [Crier](/prow/cmd/crier) which reports job results back to gerrit.
