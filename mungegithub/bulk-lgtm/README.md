# Bulk LGTM Tool.

A simple tool for scanning multiple short PRs at once and LGTM lots of PRs
in bulk.

## Usage
For now, this is a single-user tool, it is *not* suitable for running on the
public internet.

You should run it yourself with:
```sh
cd mungegithub
go build mungegithub.go
./mungegithub --organization=kubernetes \
    --project=kubernetes \
    --pr-mungers=bulk-lgtm \
    --token-file=/path/to/your/github/token.txt \
    --bulk-lgtm-github-user \
    --address="127.0.0.1:8080" \
    --www=$PWD/bulk-lgtm/www \
    --dry-run=false
```

Once that is running, visit http://localhost:8080/

### Configuring
There are three flags to configure the behavior of the tool:
   * `--bulk-lgtm-max-diff` Maximum size of a PR
   * `--bulk-lgtm-max-changed-files` Maximum number of different files changed
   * `--bulk-lgtm-max-commits` Maximum number of commits

All PRs that don't match the above limits are skipped. 
