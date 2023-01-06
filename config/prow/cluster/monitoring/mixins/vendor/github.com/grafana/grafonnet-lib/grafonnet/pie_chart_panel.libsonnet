{
  /**
   * Returns a new pie chart panel that can be added in a row.
   * It requires the pie chart panel plugin in grafana, which needs to be explicitly installed.
   *
   * @name pieChartPanel.new
   *
   * @param title The title of the pie chart panel.
   * @param description Description of the panel
   * @param span Width of the panel
   * @param min_span Min span
   * @param datasource Datasource
   * @param aliasColors Define color mappings
   * @param pieType Type of pie chart (one of pie or donut)
   * @param showLegend Show legend
   * @param showLegendPercentage Show percentage values in the legend
   * @param legendType Type of legend (one of 'Right side', 'Under graph' or 'On graph')
   * @param valueName Type of tooltip value
   * @param repeat Variable used to repeat the pie chart
   * @param repeatDirection Which direction to repeat the panel, 'h' for horizontal and 'v' for vertical
   * @param maxPerRow Number of panels to display when repeated. Used in combination with repeat.
   * @return A json that represents a pie chart panel
   */
  new(
    title,
    description='',
    span=null,
    min_span=null,
    datasource=null,
    height=null,
    aliasColors={},
    pieType='pie',
    valueName='current',
    showLegend=true,
    showLegendPercentage=true,
    legendType='Right side',
    repeat=null,
    repeatDirection=null,
    maxPerRow=null,
  ):: {
    type: 'grafana-piechart-panel',
    [if description != null then 'description']: description,
    pieType: pieType,
    title: title,
    aliasColors: aliasColors,
    [if span != null then 'span']: span,
    [if min_span != null then 'minSpan']: min_span,
    [if height != null then 'height']: height,
    [if repeat != null then 'repeat']: repeat,
    [if repeatDirection != null then 'repeatDirection']: repeatDirection,
    [if maxPerRow != null then 'maxPerRow']: maxPerRow,
    valueName: valueName,
    datasource: datasource,
    legend: {
      show: showLegend,
      values: true,
      percentage: showLegendPercentage,
    },
    legendType: legendType,
    targets: [
    ],
    _nextTarget:: 0,
    addTarget(target):: self {
      local nextTarget = super._nextTarget,
      _nextTarget: nextTarget + 1,
      targets+: [target { refId: std.char(std.codepoint('A') + nextTarget) }],
    },
  },
}
