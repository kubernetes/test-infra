{
  /**
   * Returns a new table panel that can be added in a row.
   * It requires the table panel plugin in grafana, which is built-in.
   *
   * @name table.new
   *
   * @param title The title of the graph panel.
   * @param span Width of the panel
   * @param height Height of the panel
   * @param description Description of the panel
   * @param datasource Datasource
   * @param min_span Min span
   * @param styles Styles for the panel
   * @param columns Columns for the panel
   * @param sort Sorting instruction for the panel
   * @param transform allow table manipulation to present data as desired
   * @param transparent Boolean (default: false) If set to true the panel will be transparent
   * @param links Set of links for the panel.
   * @return A json that represents a table panel
   */
  new(
    title,
    description=null,
    span=null,
    min_span=null,
    height=null,
    datasource=null,
    styles=[],
    transform=null,
    transparent=false,
    columns=[],
    sort=null,
    time_from=null,
    time_shift=null,
    links=[],
  ):: {
    type: 'table',
    title: title,
    [if span != null then 'span']: span,
    [if min_span != null then 'minSpan']: min_span,
    [if height != null then 'height']: height,
    datasource: datasource,
    targets: [
    ],
    styles: styles,
    columns: columns,
    timeFrom: time_from,
    timeShift: time_shift,
    links: links,
    [if sort != null then 'sort']: sort,
    [if description != null then 'description']: description,
    [if transform != null then 'transform']: transform,
    [if transparent == true then 'transparent']: transparent,
    _nextTarget:: 0,
    addTarget(target):: self {
      local nextTarget = super._nextTarget,
      _nextTarget: nextTarget + 1,
      targets+: [target { refId: std.char(std.codepoint('A') + nextTarget) }],
    },
    addTargets(targets):: std.foldl(function(p, t) p.addTarget(t), targets, self),
    addColumn(field, style):: self {
      local style_ = style { pattern: field },
      local column_ = { text: field, value: field },
      styles+: [style_],
      columns+: [column_],
    },
    hideColumn(field):: self {
      styles+: [{
        alias: field,
        pattern: field,
        type: 'hidden',
      }],
    },
    addLink(link):: self {
      links+: [link],
    },
  },
}
