{
  /**
   * @name loki.target
   */
  target(
    expr,
    hide=null,
  ):: {
    [if hide != null then 'hide']: hide,
    expr: expr,
  },
}
