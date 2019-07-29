# Configurator

Configurator takes some YAML Testgrid config files and (optionally) a Prow configuration and generates
a complete Testgrid configuration. 

This utility is important for the [inner workings](/testgrid/build_test_update.md) of Testgrid, but if
you're looking to just add to or modify an existing configuration, read [`config.md`]
instead.

## Basic Usage

`configurator --yaml=config.yaml --print-text --oneshot` will read the configuration from the YAML
file and print it to standard output for humans to read.

If `--oneshot` is omitted, it will do the same thing and continue running. If the configuration it's
reading is modified, it will generate again.

Instead of `--print-text`, you can just `--validate-config-file`, or specify an `--output`.

```bash
--output=/path/outputfile     # Writes the generated configuration to that file
--output=gcs://bucket/object  # Writes the generated configuration to a GCS bucket. Credentials are needed.
```

`--default` specifies default settings to use whenever a setting isn't specified in the YAML configuration.

## Usage with Prow

If Testgrid is running in parallel with [Prow], configuration can be annotated to a Prow job instead
of separately configured in a YAML file. Details for how to write these annotations are in [`config.md`].

The options `--prow-config` and `--prow-job-config` are used to specify where the Prow configurations are.
They must be specified together.

## Deserialization Options

Configurator reads YAML configurations. Testgrid itself expects its configuration to be formatted as
a [protocol buffer][`config.proto`], and has no concept of a YAML configuration.

By default, Configurator outputs a [`config.proto`], since it usually serves configurations to Testgrid.
However, if you want to use Configurator to generate output that will be consumed by a _different
instance_ of Configurator, you may want to use the `--output-yaml` option.

For example, if you are running your own instance of Prow, want to use annotations,
and need to output those annotations so that the k8s instance of Configurator can parse them correctly,
you may want to use a command like the following:

```bash
configurator \
    --yaml=./dashboard_list.yaml \
    --default=./project_defaults.yaml \
    --prow-config=./prow_config.yaml \
    --prow-job-config=./prow_jobs \
    --output=./output_config.yaml \
    --output-yaml \
    --oneshot
```

[`config.proto`]: /testgrid/config/config.proto
[`config.md`]: /testgrid/config.md
[Prow]: /prow/README.md