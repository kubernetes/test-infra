#standardSQL
select /* Find jobs that have not passed in a long time */
  jobs.job,
  latest_pass, /* how recently did this job pass */
  weekly_builds,  /* how many times a week does it run */
  first_run, /* when is the first time it ran */
  latest_run  /* when is the most recent run */
from (
  select /* filter to jobs that ran this week */
    job,
    count(1) weekly_builds
  from `k8s-gubernator.build.all`
  where
    started > timestamp_sub(current_timestamp(), interval 7 day)
  group by job
  order by job
) jobs
left join (
  select /* find the most recent time each job passed (may not be this week) */
    job,
    max(started) latest_pass
  from `k8s-gubernator.build.all`
  where
    result = 'SUCCESS'
  group by job
) passes
on jobs.job = passes.job
left join (
  select /* find the oldest, newest run of each job */
    job,
    date(min(started)) first_run,
    date(max(started)) latest_run
  from `k8s-gubernator.build.all`
  group by job
) runs
on jobs.job = runs.job
where
  latest_pass is null /* that never passed */
  and date_diff(current_date, first_run, month) > 1 /* running for more than a month */
  and date_diff(current_date, latest_run, day) < 7 /* ran this week */
order by latest_pass, first_run, weekly_builds desc, jobs.job
