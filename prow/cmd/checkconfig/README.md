# `checkconfig`

`checkconfig` loads the Prow configuration given with `--config-path`,
`--job-config-path` and `--plugin-config` in order to validate it.
Use `checkconfig` as a pre-submit for any repository holding Prow
configuration to ensure that check-ins do not break anything.
