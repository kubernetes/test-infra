{
  local grafanaDashboards = super.grafanaDashboards,

  // Automatically add a uid to each dashboard based on the base64 encoding
  // of the file name and set the timezone to be 'default'.
  grafanaDashboards:: {
    [filename]: grafanaDashboards[filename] {
      uid: std.md5(filename),
      timezone: '',

      // Modify tooltip to only show a single value
      rows: [
        row {
          panels: [
            panel {
              tooltip+: {
                shared: false,
              },
            }
            for panel in super.panels
          ],
        }
        for row in super.rows
      ],

    }
    for filename in std.objectFields(grafanaDashboards)
  },
}
