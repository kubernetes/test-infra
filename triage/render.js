"use strict";

var rightArrow = "\u25ba";
var downArrow = "\u25bc";

const kGraphHeight = 200;      // keep synchronized with style.css:div.graph
const kCollapseThreshold = 5;  // maximum number of entries before being collapsed

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

function spyglassURLForBuild(build, test) {
  let buildPath = builds.jobPaths[build.job] + '/' + build.number;
  var spyglassURL = 'https://prow.k8s.io/view/gcs/' + buildPath.slice(5);
  if (build.pr) {
    spyglassURL = spyglassURL.replace(/(\/pr-logs\/pull\/)[^/]*\//, '$1' + build.pr + '/');
  }
  return spyglassURL;
}

// Turn a build object into a link with information.
function buildToHtml(build, test, skipNumber) {
  let started = tsToString(build.started);
  return `<a href="${spyglassURLForBuild(build, test)}" target="_blank" rel="noopener">${skipNumber ? "" : build.number} ${started}</a>`;
}

// Turn a job and array of build numbers into a list of build links.
function buildNumbersToHtml(job, buildNumbers, test) {
  var buildCount = builds.count(job);
  var pct = buildNumbers.length / builds.count(job);
  var out = `Failed in ${Math.round(pct * 100)}% (${buildNumbers.length}/${buildCount}) of builds: <ul>`;
  for (let number of buildNumbers) {
    out += '\n<li>' + buildToHtml(builds.get(job, number), test);
  }
  out += '\n</ul>';
  return out;
}

// Append a list item containing information about a job's runs.
function addBuildListItem(jobList, job, buildNumbers, hits, test) {
  var jobEl = addElement(jobList, 'li', null, [sparkLineSVG(hits), ` ${buildNumbers.length} ${job} ${rightArrow}`,
    createElement('p', {
      style: {display: 'none'},
      dataset: {job: job, test: test || '', buildNumbers: JSON.stringify(buildNumbers)},
    })
  ]);
}

// Render a list of builds as a list of jobs with expandable build sections.
function renderJobs(parent, clusterId) {
  if (parent.children.length > 0) {
    return;  // already done
  }

  var counts = clustered.makeCounts(clusterId);

  var jobs = {};
  for (let build of clustered.buildsForClusterById(clusterId)) {
    let job = build.job;
    if (!jobs[job]) {
      jobs[job] = new Set();
    }
    jobs[job].add(build.number);
  }

  var jobs = Object.entries(jobs);
  jobs = sortByKey(jobs, j => [-dayCounts(counts[j[0]]), -j[1].size]);

  var jobAllSection = false;
  var dayCount = dayCounts(counts['']);
  var countSum = 0;

  var jobList = addElement(parent, 'ul');
  for (let [job, buildNumbersSet] of jobs) {
    // This sort isn't strictly correct - our numbers are too large - but in practice we shouldn't
    // have ID numbers this similar anyway, and it's not fatal if builds this close together
    // are sorted wrong.
    let buildNumbers = Array.from(buildNumbersSet).sort((a,b) => Number(b) - Number(a));
    var count = counts[job];
    if (jobs.length > kCollapseThreshold && !jobAllSection && countSum > 0.8 * dayCount) {
      addElement(jobList, 'button', {className: 'rest', title: 'Show Daily Bottom 20%'}, 'More');
      addElement(jobList, 'hr');
      jobAllSection = true;
    }
    countSum += dayCounts(count);
    addBuildListItem(jobList, job, buildNumbers, count);
  }
}

// Return an SVG path displaying the given histogram arr, with width
// being per element and height being the total height of the graph.
function sparkLinePath(arr, width, height) {
  var max = 0;
  for (var i = 0; i < arr.length; i++) {
    if (arr[i] > max)
      max = arr[i];
  }
  var scale = max > 0 ? height / max : 1;

  // Full documentation here: https://www.w3.org/TR/SVG/paths.html#PathData
  // Basics:
  // 0,0 is the top left corner
  // Commands:
  //    M x y: move to x, y
  //    h dx: move horizontally +/- dx
  //    V y: move vertically to y
  // Here, we're drawing a histogram as a single polygon with right angles.
  var out = 'M0,' + height;
  var x = 0, y = height;
  for (var i = 0; i < arr.length; i++) {
    var h = height - Math.ceil(arr[i] * scale);
    if (h != y) {
      // h2V0 draws horizontally across, then a line to the top of the canvas.
      out += `h${i * width - x}V${h}`;
      x = i * width;
      y = h;
    }
  }
  out += `h${arr.length * width - x}`;
  if (y != height)
    out += `V${height}`;

  return out;
}

function sparkLineSVG(arr) {
  var width = 4;
  var height = 16;
  var path = sparkLinePath(arr, width, height);
  return createElement('span', {
    dataset: {tooltip: 'hits over last week, newest on the right'},
    innerHTML: `<svg height=${height} width='${(arr.length) * width}'><path d="${path}" /></svg>`,
  });
}

function dayCounts(arr) {
  var l = arr.length;
  return arr[l-1]+arr[l-2]+arr[l-3]+arr[l-4];
}

function renderLatest(el, keyId) {
  var ctxs = [];
  for (let ctx of clustered.buildsWithContextForClusterById(keyId)) {
    ctxs.push(ctx);
  }
  ctxs.sort((a, b) => { return (b[0].started - a[0].started) || (b[2] < a[2]); })
  var n = 0;
  addElement(el, 'tr', null, [
    createElement('th', null, 'Time'),
    createElement('th', null, 'Job'),
    createElement('th', null, 'Test')
  ]);
  var buildsEmittedSet = new Set();
  var buildsEmitted = [];
  var n = 0;
  for (let [build, job, test] of ctxs) {
    var key = job + build.number;
    if (buildsEmittedSet.has(key)) continue;
    buildsEmittedSet.add(key);
    buildsEmitted.push([build, job, test]);
    addElement(el, 'tr', null, [
      createElement('td', {innerHTML: `${buildToHtml(build, test, true)}`}),
      createElement('td', null, job),
      createElement('td', null, test),
    ]);
    if (++n >= 5) break;
  }
  return buildsEmitted;
}

// Return a list of strings and spans made from text according to spans.
// Spans is a list of [text segment length, span segment length, ...] repeating.
function renderSpans(text, spans) {
  if (!spans) {
    return [text];
  }
  if (spans.length > 1000) {
    console.warn(`Not highlighting excessive number of spans to avoid browser hang: ${spans.length}`);
    return [text];
  }
  var out = [];
  var c = 0;
  for (var i = 0; i < spans.length; i += 2) {
    out.push(text.slice(c, c + spans[i]));
    c += spans[i];
    if (i + 1 < spans.length) {
      out.push(createElement('span',
                             {className: 'mm', title: 'not present in all failure messages'},
                             text.slice(c, c + spans[i+1])));
      c += spans[i + 1];
    }
  }
  return out;
}

function makeGitHubIssue(id, text, owner, latestBuilds) {
  let title = `Failure cluster [${id.slice(0, 8)}...]`;
  let body = `### Failure cluster [${id}](https://go.k8s.io/triage#${id})

##### Error text:
\`\`\`
${text.slice(0, Math.min(text.length, 1500))}
\`\`\`
#### Recent failures:
`;
  for (let [build, job, test] of latestBuilds) {
    const url = spyglassURLForBuild(build, test);
    const started = tsToString(build.started);
    body += `[${started} ${job}](${url})\n`
  }
  body += `\n\n/kind failing-test`;
  body += '\n<!-- If this is a flake, please add: /kind flake -->';
  if (owner) {
    body += `\n\n/sig ${owner}`;
  } else {
    body += '\n\n<!-- Please assign a SIG using: /sig SIG-NAME -->';
  }
  return [title, body];
}

// Render a section for each cluster, including the text, a graph, and expandable sections
// to dive into failures for each test or job.
function renderCluster(top, cluster) {
  let {key, id, text, tests, spans, owner} = cluster;

  function plural(count, word, suffix) {
    return count + ' ' + word + (count == 1 ? '' : suffix);
  }

  function swapArrow(el) {
    el.textContent = el.textContent.replace(downArrow, rightArrow);
  }

  var counts = clustered.makeCounts(id);

  var clusterSum = clustersSum(tests);
  var todayCount = clustered.getHitsInLastDayById(id);
  var ownerTag = createElement('span', {className: 'owner sig-' + (owner || ''), dataset: {tooltip: 'inferred owner'}});
  var fileBug = createElement('a', {href: '#', target: '_blank', rel: 'noopener'}, 'file bug');
  var failureNode = addElement(top, 'div', {id: id, className: 'failure'}, [
    createElement('h2', null, [
      `${plural(clusterSum, 'test failure', 's')} (${todayCount} today) look like `,
      createElement('a', {href: '#' + id}, 'link'),
      createElement('a', {href: 'https://github.com/search?type=Issues&q=org:kubernetes%20' + id, target: '_blank', rel: 'noopener'}, 'search github'),
      fileBug,
      ownerTag,
    ]),
    createElement('pre', null, options.showNormalize ? key : renderSpans(text, spans)),
    createElement('div', {className: 'graph', dataset: {cluster: id}}),
  ]);

  if (owner) {
    ownerTag.innerText = owner;
  } else {
    ownerTag.remove();
  }

  var latest = createElement('table');
  var list = addElement(failureNode, 'ul', null, [
    createElement('span', null, [`Latest Failures`, latest]),
  ]);

  var latestBuilds = renderLatest(latest, id);

  fileBug.addEventListener('click', () => {
    let [title, body] = makeGitHubIssue(id, text, owner, latestBuilds);
    title = encodeURIComponent(title);
    body = encodeURIComponent(body);
    fileBug.href = `https://github.com/kubernetes/kubernetes/issues/new?body=${body}&title=${title}`;
  })

  var clusterJobs = addElement(list, 'li');

  var jobSet = new Set();

  var testList = createElement('ul');

  var expander = addElement(list, 'li', null, [`Failed in ${plural(tests.length, 'Test', 's')} ${downArrow}`, testList]);

  // If we expanded all the tests and jobs, how many rows would it take?
  var jobCount = sum(tests, t => t.jobs.length);

  // Sort tests by descending [last day hits, total hits]
  var testsSorted = sortByKey(tests, t => [-dayCounts(counts[t.name]), -sum(t.jobs, j => j.builds.length)]);

  var allTestsDayCount = dayCounts(counts['']);
  var testsDayCountSum = 0;

  var allSection = false;
  var testsShown = 0;
  var i = 0;

  for (var test of testsSorted) {
    i++;
    var testCount = sum(test.jobs, j => j.builds.length);

    var testDayCount = dayCounts(counts[test.name]);

    if (tests.length > kCollapseThreshold) {
      if (!allSection && testsDayCountSum > 0.8 * allTestsDayCount) {
        testsShown = i;
        addElement(testList, 'button', {className: 'rest', title: 'Show Daily Bottom 20% Tests'}, 'More');
        addElement(testList, 'hr');
        allSection = true;
      }
    }

    var testDayCountSum = 0;
    testsDayCountSum += testDayCount;

    var el = addElement(testList, 'li', null, [
      sparkLineSVG(counts[test.name]),
      ` ${testCount} ${test.name} ${rightArrow}`,
    ]);

    var jobList = addElement(el, 'ul', {style: {display: 'none'}});

    var jobs = sortByKey(test.jobs, j => [-dayCounts(counts[j.name + ' ' + test.name]), -j.builds.length]);

    var jobAllSection = false;

    var j = 0;
    for (var job of jobs) {
      var jobCount = counts[job.name + ' ' + test.name];
      if (jobs.length > kCollapseThreshold && !jobAllSection && testDayCountSum > 0.8 * testDayCount) {
        addElement(jobList, 'button', {className: 'rest', title: 'Show Daily Bottom 20% Jobs'}, 'More');
        addElement(jobList, 'hr');
        jobAllSection = true;
      }
      jobSet.add(job.name);
      addBuildListItem(jobList, job.name, job.builds, jobCount, test.name);
      testDayCountSum += dayCounts(jobCount);
    }
  }

  if ((testsShown === 0 && tests.length > kCollapseThreshold) || testsShown > kCollapseThreshold) {
    testList.style.display = 'none';
    swapArrow(expander.firstChild);
  }

  clusterJobs.innerHTML = `Failed in ${plural(jobSet.size, 'Job', 's')} ${rightArrow}<div style="display:none" class="jobs" data-cluster="${id}">`;
  if (jobSet.size <= 10) {  // automatically expand small job lists to save clicking
    expand(clusterJobs.children[0]);
  }

  return 1;
}

// Convert an array of integers into a histogram array of two-element arrays.
function makeBuckets(hits, width, start, end) {
  var cur = start;
  cur -= (cur % width);  // align to width
  var counts = new Uint32Array(Math.floor((end - cur) / width) + 1);
  for (var hit of hits) {
    counts[Math.floor((hit - cur) / width)] += 1;
  }
  var buckets = [];
  for (var c of counts) {
    buckets.push([cur, c]);
    cur += width;
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
  if (target.nodeName === "BUTTON" && target.className === "rest") {
    target.remove();
    return true;
  }
  while (target.nodeName !== "LI" && target.parentNode) {
    target = target.parentNode;
  }
  var text = target.childNodes[target.childNodes.length - 2];
  var child = target.children[target.children.length - 1];
  if (target.nodeName == "LI" && child && text) {
    if (text.textContent.includes(rightArrow)) {
      text.textContent = text.textContent.replace(rightArrow, downArrow);
      child.style = "";

      if (child.dataset.buildNumbers) {
        var job = child.dataset.job;
        var test = child.dataset.test;
        var buildNumbers = JSON.parse(child.dataset.buildNumbers);
        child.innerHTML = buildNumbersToHtml(job, buildNumbers, test);
      } else if (child.dataset.cluster) {
        var cluster = child.dataset.cluster;
        if (child.className === 'graph') {
          renderGraph(child, clustered.buildsForClusterById(cluster));
        } else if (child.className === 'jobs') {
          renderJobs(child, cluster);
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
    sparkLinePath: sparkLinePath,
  }
}
