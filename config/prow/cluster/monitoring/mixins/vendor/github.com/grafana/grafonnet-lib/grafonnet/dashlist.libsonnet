{
  /**
   * Returns a new dashlist panel that can be added in a row.
   * It requires the dashlist panel plugin in grafana, which is built-in.
   *
   * @name dashlist.new
   *
   * @param title The title of the dashlist panel.
   * @param description Description of the panel
   * @param query Query to search by
   * @param tags Tag(s) to search by
   * @param recent Displays recently viewed dashboards
   * @param search Description of the panel
   * @param starred Displays starred dashboards
   * @param headings Chosen list selection(starred, recently Viewed, search) is shown as a heading
   * @param limit Set maximum items in a list
   * @return A json that represents a dashlist panel
   */
  new(
    title,
    description=null,
    query=null,
    tags=[],
    recent=true,
    search=false,
    starred=false,
    headings=true,
    limit=10,
  ):: {
    type: 'dashlist',
    title: title,
    query: if query != null then query else '',
    tags: tags,
    recent: recent,
    search: search,
    starred: starred,
    headings: headings,
    limit: limit,
    [if description != null then 'description']: description,
  },
}
