# Experiment

This is a catchall directory for small shard projects that are not supported
and do not necessarily meet the standards of the rest of the repo. They are
most often in the form of scripts, or tools intended for one-shot or limited
use.

If you feel like an experiment has outgrown this definition, please
open a PR to move it into its own directory.

If you feel like an experiment is now defunct and unused, please open a PR to
remove it.

## Python Scripts

Most of python scripts located in this directory can be ran straight by invoking
the script in a terminal. For a deterministic outcome, try use make rules, for
example to run `flakedetector.py`, do:

```
make -C experiment ARGS=<EXTRA-ARGS> run-flakedetector
```
