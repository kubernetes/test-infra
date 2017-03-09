"use strict";

var rightArrow = "\u25ba";
var downArrow = "\u25bc";

const kGraphHeight = 200;     // keep synchronized with style.css:div.graph

if (Object.entries === undefined) {
  // Simple polyfill for Safari compatibility.
  // Object.entries is an ES2017 feature.
  Object.entries = function(obj) {
    var ret = [];
    for (let key of Object.keys(obj)) {
      ret.push([key, obj[key]]);
    }
    return ret;
  }
}

if (typeof Element !== 'undefined') {
  // Useful extension for DOM nodes.
  Element.prototype.removeChildren = function() {
    while (this.firstChild) {
      this.removeChild(this.firstChild);
    }
  }
}

// Create a new DOM node of `type` with `opts` attributes and with given children.
// If children is a string, or an array with string elements, they become text nodes.
function createElement(type, opts, children) {
  var el = document.createElement(type);
  if (opts) {
    for (let [key, value] of Object.entries(opts)) {
      if (typeof value === "object") {
        for (let [subkey, subvalue] of Object.entries(value)) {
          el[key][subkey] = subvalue;
        }
      } else {
        el[key] = value;
      }
    }
  }
  if (children) {
    if (typeof children === "string") {
      el.textContent = children;
    } else {
      for (let child of children) {
        if (typeof child === "string")
          child = document.createTextNode(child);
        el.appendChild(child);
      }
    }
  }
  return el;
}

// Like createElement, but also appends the new node to parent's children.
function addElement(parent, type, opts, children) {
  var el = createElement(type, opts, children);
  parent.appendChild(el);
  return el;
}

// Turn a build object into a link with information.
function buildToHtml(build) {
  let started = new Date(build.started * 1000).toLocaleString();
  let buildPath = builds.jobPaths[build.job] + '/' + build.number;
  var gubernatorURL = 'https://k8s-gubernator.appspot.com/build/' + buildPath.slice(5);
  if (build.pr) {
    gubernatorURL = gubernatorURL.replace(/(\/pr-logs\/pull\/)[^/]*\//, '$1' + build.pr + '/');
  }
  return `<a href="${gubernatorURL}">${build.number} ${started}</a>`;
}

// Turn a job and array of build numbers into a list of build links.
function buildNumbersToHtml(job, buildNumbers) {
  var buildCount = builds.count(job);
  var pct = buildNumbers.length / builds.count(job);
  var out = `Failed in ${Math.round(pct * 100)}% (${buildNumbers.length}/${buildCount}) of builds: <ul>`;
  out += ''
  for (let number of buildNumbers) {
    out += '\n<li>' + buildToHtml(builds.get(job, number));
  }
  out += '\n</ul>';
  return out;
}

// Append a list item containing information about a job's runs.
function addBuildListItem(jobList, job, buildNumbers) {
  var jobEl = addElement(jobList, 'li', null, `${buildNumbers.length} ${job} ${rightArrow}`);
  var p = addElement(jobEl, 'p', {
      style: {display: 'none'},
      dataset: {job: job, buildNumbers: JSON.stringify(buildNumbers)},
  });
}

// Render a list of builds as a list of jobs with expandable build sections.
function renderJobs(parent, buildsIterator) {
  var clusterJobs = {};
  for (let build of buildsIterator) {
    let job = build.job;
    if (!clusterJobs[job]) {
      clusterJobs[job] = new Set();
    }
    clusterJobs[job].add(build.number);
  }

  var clusterJobs = Object.entries(clusterJobs);
  clusterJobs.sort();

  var jobList = addElement(parent, 'ul');
  for (let [job, buildNumbersSet] of clusterJobs) {
    let buildNumbers = Array.from(buildNumbersSet).sort();
    addBuildListItem(jobList, job, buildNumbers);
  }
}

// Render a section for each cluster, including the text, a graph, and expandable sections
// to dive into failures for each test or job.
function renderCluster(top, key, keyId, text, clusters) {
  var clusterSum = clustersSum(clusters);
  var recentCount = clustered.getHitsInLastDay(keyId);
  var failureNode = addElement(top, 'div', {id: keyId}, [
    createElement('h2',
      {innerHTML: `${clusterSum} FAILURE${clusterSum > 1 ? "S" : ""} (${recentCount} RECENT) MATCHING <a href="#${keyId}" class="key">${keyId}</a>`}),
    createElement('pre', null, options.showNormalize ? key : text),
    createElement('div', {className: 'graph', dataset: {cluster: keyId}}),
  ]);
  var clusterJobs = addElement(failureNode, 'ul');
  var list = addElement(failureNode, 'ul');

  var jobSet = new Set();

  for (var [testName, testsGrouped] of clusters) {
    var testCount = sum(testsGrouped, t => t[1].length);
    var el = addElement(list, 'li', null, `${testCount} ${testName} ${rightArrow}`);
    var jobList = addElement(el, 'ul');
    if (clusterSum > 5) {
      jobList.style.display = 'none';
    }
    for (var [job, buildNumbers] of testsGrouped) {
      jobSet.add(job);
      addBuildListItem(jobList, job, buildNumbers);
    }
  }

  clusterJobs.innerHTML = `<li>${jobSet.size} Jobs ${rightArrow}<div style="display:none" class="jobs" data-cluster="${keyId}">`;
  if (jobSet.size <= 10) {  // automatically expand small job lists to save clicking
    expand(clusterJobs.children[0]);
  }

  return 1;
}

// Convert a sorted array of integers into a histogram array of two-element arrays.
function makeBuckets(hits, width, start, end) {
  // Bucket into 4 hour chunks
  var cur = start;
  cur -= (cur % width);
  var buckets = [[cur, 0]];
  for (var hit of hits) {
    while (hit >= cur + width) {
      cur += width;
      buckets.push([cur, 0]);
    }
    buckets[buckets.length - 1][1] += 1;
  }
  while (cur + width <= end) {
    cur += width;
    buckets.push([cur, 0]);
  }
  return buckets;
}

// Display a line graph on `element` showing failure occurrences.
function renderGraph(element, buildsIterator) {
  // Defer rendering until later if the Charts API isn't available.
  if (!google.charts.loaded) {
    setTimeout(() => renderGraph(element, buildsIterator), 100);
    return;
  }

  // Find every build time in the current cluster.
  var hits = [];
  var buildsSeen = new Set();
  var buildTimes = [];  // one for each build
  for (let build of buildsIterator) {
    hits.push(build.started);
    let buildKey = build.job + build.number;
    if (!buildsSeen.has(buildKey)) {
      buildsSeen.add(buildKey);
      buildTimes.push(build.started);
    }
  }
  hits.sort();
  buildTimes.sort();

  var width = 60 * 60; // Bucket into 1 hour chunks
  var widthRecip = 60 * 60 / width;
  var hitBuckets = makeBuckets(hits, width, builds.timespan[0], builds.timespan[1]);
  var buildBuckets = makeBuckets(buildTimes, width, builds.timespan[0], builds.timespan[1]);
  var buckets = buildBuckets.map((x, i) => [new Date(x[0] * 1000), x[1] * widthRecip, hitBuckets[i][1] * widthRecip]);

  var data = new google.visualization.DataTable();
  data.addColumn('date', 'X');
  data.addColumn('number', 'Builds');
  data.addColumn('number', 'Tests');
  data.addRows(buckets);

  var formatter = new google.visualization.DateFormat({'pattern': 'yyyy-MM-dd HH:mm z'});
  formatter.format(data, 0);

  var options = {
    width: 1200,
    height: kGraphHeight,
    hAxis: {title: 'Time', format: 'M/d'},
    vAxis: {title: 'Failures per hour'},
    legend: {position: 'none'},
    focusTarget: 'category',
  };

  var chart = new google.visualization.LineChart(element);
  chart.draw(data, options);
}

// When someone clicks on an expandable element, render the sub content as necessary.
function expand(target) {
  var child = target.children[0];
  var text = target.childNodes[0];
  if (target.nodeName == "LI" && child && text) {
    if (text.textContent.includes(rightArrow)) {
      text.textContent = text.textContent.replace(rightArrow, downArrow);
      child.style = "";

      if (child.dataset.buildNumbers) {
        var job = child.dataset.job;
        var buildNumbers = JSON.parse(child.dataset.buildNumbers);
        child.innerHTML = buildNumbersToHtml(job, buildNumbers);
      } else if (child.dataset.cluster) {
        var cluster = child.dataset.cluster;
        if (child.className === 'graph') {
          renderGraph(child, clustered.buildsForCluster(cluster));
        } else if (child.className === 'jobs') {
          renderJobs(child, clustered.buildsForCluster(cluster));
        }
      }

      return true;
    } else if (text.textContent.includes(downArrow)) {
      text.textContent = text.textContent.replace(downArrow, rightArrow);
      child.style = "display: none";
      return true;
    }
  }
  return false;
}

if (typeof module !== 'undefined' && module.exports) {
  // enable node.js `require` to work for testing
  module.exports = {
    makeBuckets: makeBuckets,
  }
}
