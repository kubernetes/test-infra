#standardSQL
select
  job,
  build_consistency,
  commit_consistency,
  flakes,
  runs,
  commits,
  array(
    select as struct
      i.n name,
      count(i.failures) flakes
    from tt.tests i
    group by name
    having name not in ('Test', 'DiffResources')
    order by flakes desc
    limit 3
  ) flakiest
from (
select
  job,
  round(sum(if(flaked=1,passed,runs))/sum(runs),3) build_consistency,
  round(1-sum(flaked)/sum(runs),3) commit_consistency,
  sum(flaked) flakes,
  sum(runs) runs,
  count(distinct commit) commits,
  array_concat_agg(tests) tests
from (
select
  job,
  stamp,
  num,
  commit,
  if(passed = runs or passed = 0, 0, 1) flaked,
  passed,
  safe_cast(runs as int64) runs,
  array(
    select as struct
      i.name n,
      countif(i.failed) failures
    from tt.tests i
    group by n
    having failures > 0 and failures < tt.runs
    order by failures desc
  ) tests
from (
select
  job,
  max(stamp) stamp,
  num,
  if(kind = 'pull', commit, version) commit,
  sum(if(result='SUCCESS',1,0)) passed,
  count(result) runs,
  array_concat_agg(test) tests
from (
SELECT
  job,
  regexp_extract(path, r'pull/(\d+)') as num,
  if(substr(job, 0, 3) = 'pr:', 'pull', 'ci') kind,
  version,
  regexp_extract(
    (
      select i.value
      from t.metadata i
      where i.key = 'repos'
    ),
    r'[^,]+,\d+:([a-f0-9]+)"') commit,
  date(started) stamp,
  date_trunc(date(started), week) wk,
  result,
  test
FROM `k8s-gubernator.build.all` as t
where
  datetime(started) > datetime_sub(current_datetime(), interval 7 DAY)
  and version != 'unknown'
  and (
    exists(
      select as struct
        i
      from t.metadata i
      where i.key = 'repos')
    or substr(job, 0, 3) = 'ci-'))
group by job, num, commit
) as tt
) as tt
group by job
order by flakes desc, build_consistency, commit_consistency, job
) as tt
order by flakes desc, build_consistency, commit_consistency, job
