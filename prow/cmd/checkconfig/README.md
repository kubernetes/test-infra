# `checkconfig`

`checkconfig` loads the Prow configuration given with `--config-path` and `--job-config-path` in order to validate it. Use
`checkconfig` as a pre-submit for any repository holding Prow configuration to ensure that check-ins do not break anything.