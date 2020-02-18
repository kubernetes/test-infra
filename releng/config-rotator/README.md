# config-rotator

config-rotator rotated forked presubmit, periodic, and postsubmit job configs created by `config-forker`.

## Usage

* `--config-file`: Path to the job config to rotate
* `--new`: Version to rotate to (one of stable1, stable2, stable3)
* `--old`: Version to rotate from (one of beta, stable1, stable2)

## Actions taken

For the sake of clarity, all the below will assume we are rotating with `--old stable1 --new stable2`.

For all jobs:

- References to `stable1` in `args` will be replaced with `stable2`
- References to `stable1` in `command` will be replaced with `stable2`
- References to `stable1` in the job name will be replaced with `stable2`

For periodics:

- References to `stable1` in the job `tags` will replaced with `stable2`
- If `fork-per-release-periodic-interval` or `fork-per-release-cron` has a non-empty list of values, the leftmost
  value will be popped off and used to replace the current `interval` or `cron`, respectively.
