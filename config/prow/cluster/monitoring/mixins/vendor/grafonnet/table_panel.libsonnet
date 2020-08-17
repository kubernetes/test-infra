{
  /**
   * Returns a new table panel that can be added in a row.
   * It requires the table panel plugin in grafana, which is built-in.
   *
   * @param title The title of the graph panel.
   * @param span Width of the panel
   * @param description Description of the panel
   * @param datasource Datasource
   * @param min_span Min span
   * @param styles Styles for the panel
   * @return A json that represents a table panel
   */
  new(
    title,
    description=null,
    span=null,
    min_span=null,
    datasource=null,
    styles=[],
  ):: {
    type: 'table',
    title: title,
    [if span != null then 'span']: span,
    [if min_span != null then 'minSpan']: min_span,
    datasource: datasource,
    targets: [
    ],
    styles: styles,
    [if description != null then 'description']: description,
    transform: 'table',
    _nextTarget:: 0,
    addTarget(target):: self {
      // automatically ref id in added targets.
      // https://github.com/kausalco/public/blob/master/klumps/grafana.libsonnet
      local nextTarget = super._nextTarget,
      _nextTarget: nextTarget + 1,
      targets+: [target { refId: std.char(std.codepoint('A') + nextTarget) }],
    },
  },
}
