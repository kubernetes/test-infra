{
  /**
   * @name alertlist.new
   */
  new(
    title='',
    span=null,
    show='current',
    limit=10,
    sortOrder=1,
    stateFilter=[],
    onlyAlertsOnDashboard=true,
    transparent=null,
    description=null,
    datasource=null,
  )::
    {
      [if transparent != null then 'transparent']: transparent,
      title: title,
      [if span != null then 'span']: span,
      type: 'alertlist',
      show: show,
      limit: limit,
      sortOrder: sortOrder,
      [if show != 'changes' then 'stateFilter']: stateFilter,
      onlyAlertsOnDashboard: onlyAlertsOnDashboard,
      [if description != null then 'description']: description,
      datasource: datasource,
    },
}
