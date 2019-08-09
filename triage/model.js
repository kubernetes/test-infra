"use strict";

// Return the minimum and maximum value of arr.
// Doesn't cause stack overflows like Math.min(...arr).
function minMaxArray(arr) {
  var min = arr[0];
  var max = arr[0];
  for (var i = 1; i < arr.length; i++) {
    if (arr[i] < min) min = arr[i];
    if (arr[i] > max) max = arr[i];
  }
  return [min, max];
}

function tsToString(ts) {
  return new Date(ts * 1000).toLocaleString();
}

// Store information about individual builds.
class Builds {
  constructor(dict) {
    this.jobs = dict.jobs;
    this.jobPaths = dict.job_paths;
    this.cols = dict.cols;
    this.colStarted = this.cols.started;
    this.colPr = this.cols.pr;
    this.timespan = minMaxArray(this.cols.started);
    this.runCount = this.cols.started.length;
  }

  // Create a build object given a job and build number.
  get(job, number) {
    let indices = this.jobs[job];
    if (indices.constructor === Array) {
      let [start, count, base] = indices;
      if (number < start || number > start + count) {
        console.error('job ' + job + ' number ' + number + ' out of range.');
        return;
      }
      var index = base + (number - start);
    } else {
      var index = indices[number];
    }
    // Add more columns as necessary.
    // This is faster than dynamically adding properties to an object.
    return {
      job: job,
      number: number,
      started: this.colStarted[index],
      pr: this.colPr[index],
    };
  }

  // Count how many builds a job has.
  count(job) {
    let indices = this.jobs[job];
    if (indices.constructor === Array) {
      return indices[1];
    }
    return Object.keys(indices).length;
  }

  getStartTime() {
    return tsToString(this.timespan[0]);
  }

  getEndTime() {
    return tsToString(this.timespan[1]);
  }
}

function sum(arr, keyFunc) {
  if (arr.length === 0)
    return 0;
  return arr.map(keyFunc).reduce((a, b) => a + b);
}

function clustersSum(tests) {
  return sum(tests, t => sum(t.jobs, j => j.builds.length));
}

// Return arr sorted by value according to keyFunc, which
// should take an element of arr and return an array of values.
function sortByKey(arr, keyFunc) {
  var vals = arr.map((x, i) => [keyFunc(x), x]);
  vals.sort((a, b) => {
    for (var i = 0; i < a[0].length; i++) {
      let elA = a[0][i], elB = b[0][i];
      if (elA > elB) return 1;
      if (elA < elB) return -1;
    }
  });
  return vals.map(x => x[1]);
}

// Return a build for each test run that failed in the given cluster.
// Builds will be duplicated if it has multiple failed tests in the cluster.
function *buildsForCluster(entry) {
  for (let test of entry.tests) {
    for (let job of test.jobs) {
      for (let number of job.builds) {
        let build = builds.get(job.name, number);
        if (build) {
          yield build;
        }
      }
    }
  }
}

function *buildsWithContextForCluster(entry) {
  for (let test of entry.tests) {
    for (let job of test.jobs) {
      for (let number of job.builds) {
        let build = builds.get(job.name, number);
        if (build) {
          yield [build, job.name, test.name];
        }
      }
    }
  }
}

// Return the number of builds that completed in the last day's worth of data.
function getHitsInLastDay(entry) {
  if (entry.dayHits) {
    return entry.dayHits;
  }
  var minStarted = builds.timespan[1] - 60 * 60 * 24;
  var count = 0;
  for (let build of buildsForCluster(entry)) {
    if (build.started > minStarted) {
      count++;
    }
  }
  entry.dayHits = count;
  return count;
}

// Store test clusters and support iterating and refiltering through them.
class Clusters {
  constructor(clustered, clusterId) {
    this.data = clustered;
    this.length = this.data.length;
    this.sum = sum(this.data, c => clustersSum(c.tests));
    this.sumRecent = sum(this.data, c => c.dayHits || 0);
    this.byId = {};
    for (let cluster of this.data) {
      let keyId = cluster.id;
      if (!this.byId[keyId]) {
        this.byId[keyId] = cluster;
      }
    }
    if (clusterId !== undefined) {
      this.clusterId = clusterId;
    }
  }

  buildsForClusterById(clusterId) {
    return buildsForCluster(this.byId[clusterId]);
  }

  buildsWithContextForClusterById(clusterId) {
    return buildsWithContextForCluster(this.byId[clusterId]);
  }

  getHitsInLastDayById(clusterId) {
    return getHitsInLastDay(this.byId[clusterId]);
  }

  // Iterate through all builds. Can return duplicates.
  *allBuilds() {
    for (let entry of this.data) {
      yield *buildsForCluster(entry);
    }
  }

  // Return a new Clusters object, with the given filters applied.
  refilter(opts) {
    var out = [];
    for (let cluster of this.data) {
      if (opts.reText && !opts.reText.test(cluster.text)) {
        continue;
      }
      if (opts.sig && opts.sig.length && opts.sig.indexOf(cluster.owner) < 0) {
        continue;
      }
      var testsOut = [];
      for (let test of cluster.tests) {
        if (opts.reTest && !opts.reTest.test(test.name)) {
          continue;
        }
        var jobsOut = [];
        for (let job of test.jobs) {
          if (opts.reJob && !opts.reJob.test(job.name)) {
            continue;
          }
          if (job.name.startsWith("pr:")) {
            if (opts.pr) jobsOut.push(job);
          } else if (job.name.indexOf(":") === -1) {
            if (opts.ci) jobsOut.push(job);
          }
        }
        if (jobsOut.length > 0) {
          jobsOut = sortByKey(jobsOut, j => [-j.builds.length, j.name]);
          testsOut.push({name: test.name, jobs: jobsOut});
        }
      }
      if (testsOut.length > 0) {
        testsOut = sortByKey(testsOut, t => [-sum(t.jobs, j => j.builds.length)]);
        out.push(Object.assign({}, cluster, {tests: testsOut}));
      }
    }

    if (opts.sort) {
      var keyFunc = {
        total: c => [-clustersSum(c.tests)],
        message: c => [c.text],
        day: c => [-getHitsInLastDay(c), -clustersSum(c.tests)],
      }[opts.sort];
      out = sortByKey(out, keyFunc);
    }

    return new Clusters(out);
  }

  makeCounts(clusterId) {
    let start = builds.timespan[0];
    let width = 60 * 60 * 8;  // 8 hours

    function pickBucket(ts) {
      return ((ts - start) / width) | 0;
    }

    let size = pickBucket(builds.timespan[1]) + 1;

    let counts = {};

    function incr(key, bucket) {
      if (counts[key] === undefined) {
        counts[key] = new Uint32Array(size);
      }
      counts[key][bucket]++;
    }

    for (let [build, job, test] of this.buildsWithContextForClusterById(clusterId)) {
      let bucket = pickBucket(build.started);
      incr('', bucket);
      incr(job, bucket);
      incr(test, bucket);
      incr(job + " " + test, bucket);
    }

    return counts;
  }
}

if (typeof module !== 'undefined' && module.exports) {
  // enable node.js `require` to work for testing
  module.exports = {
    Builds: Builds,
    Clusters: Clusters,
  }
}
