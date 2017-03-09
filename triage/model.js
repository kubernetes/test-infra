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
    return new Date(this.timespan[0] * 1000);
  }

  getEndTime() {
    return new Date(this.timespan[1] * 1000);
  }
}

function sum(arr, keyFunc) {
  if (arr.length === 0)
    return 0;
  return arr.map(keyFunc).reduce((a, b) => a + b);
}

function clustersSum(clusters) {
  return sum(clusters, c => sum(c[1], t => t[1].length));
}

// Return arr sotred by value according to keyFunc, which
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

// Store test clusters and support iterating and refiltering through them.
class Clusters {
  constructor(clustered, sortBy) {
    this.data = clustered;
    this.length = this.data.length;
    this.sum = sum(this.data, c => clustersSum(c[3]));
    this.byId = {};
    for (let cluster of this.data) {
      let keyId = cluster[1];
      if (!this.byId[keyId]) {
        this.byId[keyId] = cluster;
      }
    }
    this.sumRecent = 0;
    if (sortBy !== undefined) {
      var keyFunc = {
        total: c => [-clustersSum(c[3])],
        message: c => [c[0]],
        day: c => [-this.getHitsInLastDay(c[1]), -clustersSum(c[3])],
      }[sortBy];
      this.data = sortByKey(this.data, keyFunc);
      if (sortBy === "day") {
        this.sumRecent = sum(this.data, c => c.dayHits || 0);
      }
    }
  }

  // Return a build for each test run that failed in the given cluster.
  // Builds will be duplicated if it has multiple failed tests in the cluster.
  *buildsForCluster(clusterId) {
    let entry = this.byId[clusterId];
    if (!entry) {
      console.warn(`no such cluster '${clusterId}' found`);
      return;
    }
    let [key, keyId, text, clusters] = entry;
    for (let [testName, testsGrouped] of clusters) {
      for (let [job, buildNumbers] of testsGrouped) {
        for (let number of buildNumbers) {
          let build = builds.get(job, number);
          if (build) {
            yield build;
          }
        }
      }
    }
  }

  getHitsInLastDay(clusterId) {
    if (this.byId[clusterId].dayHits) {
      return this.byId[clusterId].dayHits;
    }
    var minStarted = builds.timespan[1] - 60 * 60 * 24;
    var count = 0;
    for (let build of this.buildsForCluster(clusterId)) {
      if (build.started > minStarted) {
        count++;
      }
    }
    this.byId[clusterId].dayHits = count;
    return count;
  }

  // Iterate through all builds. Can return duplicates.
  *allBuilds() {
    for (let id of Object.keys(this.byId)) {
      yield *this.buildsForCluster(id);
    }
  }

  // Return a new Clusters object, with the given filters applied.
  refilter(opts) {
    var out = [];
    for (let [key, keyId, text, clusters] of this.data) {
      if (opts.reText && !opts.reText.test(text)) {
        continue;
      }
      var clustersOut = [];
      for (let [testName, testsGrouped] of clusters) {
        if (opts.reTest && !opts.reTest.test(testName)) {
          continue;
        }
        var groupOut = [];
        for (let group of testsGrouped) {
          let [job, buildNumbers] = group;
          if (opts.reJob && !opts.reJob.test(job)) {
            continue;
          }
          if (job.startsWith("pr:")) {
            if (opts.pr) groupOut.push(group);
          } else if (job.indexOf(":") === -1) {
            if (opts.ci) groupOut.push(group);
          }
        }
        if (groupOut.length > 0) {
          groupOut = sortByKey(groupOut, g => [-g[1].length, g[0]]);
          clustersOut.push([testName, groupOut]);
        }
      }
      if (clustersOut.length > 0) {
        clustersOut = sortByKey(clustersOut, c => [-sum(c[1], t => t[1].length)]);
        out.push([key, keyId, text, clustersOut]);
      }
    }
    return new Clusters(out, opts.sort);
  }
}

if (typeof module !== 'undefined' && module.exports) {
  // enable node.js `require` to work for testing
  module.exports = {
    Builds: Builds,
    Clusters: Clusters,
  }
}
