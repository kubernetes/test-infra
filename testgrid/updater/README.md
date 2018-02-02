
The testgrid updater reads results from GCS to create a state proto.

The testgrid server reads these protos, converts them to json which the
javascript UI reads and renders on the screen.

TODO(fejta): provide better documentation soon
