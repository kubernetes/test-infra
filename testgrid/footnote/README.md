# Footnote

TestGrid recognizes two types of configuration in this repository:
- A YAML Configuration File in [`config/testgrids`] defining what should show up on TestGrid
- Annotations on Prow Jobs, as outlined [here](../config.md)

[Configurator] handles this by looking at all of those YAML files,
organizing the data in the way TestGrid wants it, and sending it to TestGrid. It will _only_ do this,
as configured, to files in [`config/testgrids`] and [`config/jobs`].

Some jobs are handled by other projects, or by other instances of Prow. Footnote is a script that
takes Prow Jobs with annotations, and generates a TestGrid YAML Configuration.

## When Not To Use Footnote

If your Prow Job is in [`config/jobs`], do not use this script. [Configurator] will handle the
annotations for you.

If your instance of Prow has its own instance of Testgrid, do not use this script. Give your
instance of Testgrid an instance of Configurator instead.

[Configurator]: ../cmd/configurator
[`config/jobs`]: ../../config/jobs
[`config/testgrids`]: ../../config/testgrids