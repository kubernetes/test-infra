"use strict";

var builds = null;
var clustered = null;         // filtered clusters
var clusteredAll = null;      // all clusters
var options = null;           // user-provided in form or URL
var lastClusterRendered = 0;  // for infinite scrolling

// Escape special regex characters for putting a literal into a regex.
// http://stackoverflow.com/a/9310752/3694
RegExp.escape = function(text) {
  return text.replace(/[-[\]{}()*+?.,\\^$|#\s]/g, "\\$&");
};

// Load options from form inputs, put them in the URL, and return the options dict.
function readOptions() {
  var read = id => {
    let el = document.getElementById(id);
    if (el.type === "checkbox") return el.checked;
    if (el.type === "radio") return el.form[el.name].value;
    if (el.type === "text") {
      if (id.startsWith("filter")) {
        if (el.value === "") {
          return null;
        }
        try {
          return new RegExp(el.value, "im");
        } catch(err) {
          console.error("bad regexp", el.value, err);
          return new RegExp(RegExp.escape(el.value), "im");
        }
      } else {
        return el.value;
      }
    }
  }

  var opts = {
    ci: read('job-ci'),
    pr: read('job-pr'),
    reText: read('filter-text'),
    reJob: read('filter-job'),
    reTest: read('filter-test'),
    showNormalize: read('show-normalize'),
    sort: read('sort'),
  }

  var url = '';
  if (!opts.ci) url += '&ci=0';
  if (opts.pr) url += '&pr=1';
  for (var name of ["text", "job", "test"]) {
    var re = opts['re' + name[0].toUpperCase() + name.slice(1)];
    if (re) {
      var baseRe = re.toString().replace(/im$/, '').replace(/\\\//g, '/').slice(1, -1);
      url += '&' + name + '=' + encodeURIComponent(baseRe);
    }
  }
  if (url) {
    if (document.location.hash) {
      url += document.location.hash;
    }
    history.replaceState(null, "", "?" + url.slice(1));
  } else if (document.location.search) {
    history.replaceState(null, "", document.location.pathname + document.location.hash);
  }

  return opts;
}

// Convert querystring parameters into form inputs.
function setOptionsFromURL() {
  // http://stackoverflow.com/a/3855394/3694
  var qs = (function(a) {
    if (a == "") return {};
    var b = {};
    for (var i = 0; i < a.length; ++i)
    {
      var p=a[i].split('=', 2);
      if (p.length == 1)
        b[p[0]] = "";
      else
        b[p[0]] = decodeURIComponent(p[1].replace(/\+/g, " "));
    }
    return b;
  })(window.location.search.substr(1).split('&'));

  var write = (id, value) => {
    if (!value) return;
    var el = document.getElementById(id);
    if (el.type === "checkbox") el.checked = (value === "1");
    if (el.type === "text") el.value = value;
  }
  write('job-ci', qs.ci);
  write('job-pr', qs.pr);
  write('filter-text', qs.text);
  write('filter-job', qs.job);
  write('filter-test', qs.test);
}

// Render up to `count` clusters, with `start` being the first for consideration.
function renderSubset(start, count) {
  var top = document.getElementById('clusters');
  var n = 0;
  var shown = 0;
  for (let [key, keyId, text, clusters] of clustered.data) {
    if (n++ < start) continue;
    shown += renderCluster(top, key, keyId, text, clusters);
    lastClusterRendered = n;
    if (shown >= count) break;
  }
}

// Clear the page and reinitialize the renderer and filtering. Render a few failures.
function rerender(maxCount) {
  if (!clusteredAll) return;

  options = readOptions();
  clustered = clusteredAll.refilter(options);

  var top = document.getElementById('clusters');
  top.removeChildren();
  summary.removeChildren();

  var summaryText = `
            ${clustered.length} clusters of ${clustered.sum} failures`;

  if (clustered.sumRecent > 0) {
    summaryText += ` (${clustered.sumRecent} in last day)`;
  }

  summaryText += ` out of ${builds.runCount} builds from ${builds.getStartTime().toLocaleString()} to ${builds.getEndTime().toLocaleString()}.`

  summary.innerText = summaryText;

  if (clustered.length > 0) {
    let graph = addElement(summary, 'div');
    renderGraph(graph, clustered.allBuilds());
  }

  renderSubset(0, maxCount || 10);

  drawVisibleGraphs();
}

// Render clusters until a cluster with the given key is found, then scroll to that cluster.
// Bails out early if no cluster with the given key is known.
function renderUntilFound(keyId) {
  var el = null;
  if (!clustered.byId[keyId]) {
    return;
  }
  while ((el = document.getElementById(keyId)) === null) {
    if (lastClusterRendered >= clustered.length)
      return;
    renderSubset(lastClusterRendered, 50);
  }
  el.scrollIntoView();

  // expand the graph for the selected failure.
  drawVisibleGraphs();
}

// When the user scrolls down, render more clusters to provide infinite scrolling.
// This is important to make the first page load fast.
// Also, trigger a debounced lazy graph rendering pass.
function scrollHandler() {
  if (!clustered) return;
  if (lastClusterRendered < clustered.length) {
    var top = document.getElementById('clusters');
    if (top.getBoundingClientRect().bottom < 3 * window.innerHeight) {
      renderSubset(lastClusterRendered, 10);
    }
  }
  if (drawGraphsTimer) {
    clearTimeout(drawGraphsTimer);
  }
  drawGraphsTimer = setTimeout(drawVisibleGraphs, 50);
}

var drawGraphsTimer = null;

function drawVisibleGraphs() {
  for (let el of document.querySelectorAll('div.graph')) {
    if (el.children.length > 0) {
      continue;  // already rendered
    }
    let rect = el.getBoundingClientRect();
    if (0 <= rect.top + kGraphHeight && rect.top - kGraphHeight < window.innerHeight) {
      renderGraph(el, clustered.buildsForClusterById(el.dataset.cluster));
    }
  }
}

// If someone clicks on an expandable node, expand it!
function clickHandler(evt) {
  var target = evt.target;
  if (expand(target)) {
    evt.preventDefault();
    return false;
  }
}

// Download a file from GCS and invoke callback with the result.
// extracted/modified from kubernetes/test-infra/gubernator/static/build.js
function get(uri, callback, onprogress) {
  if (uri[0] === '/') {
    // Matches /bucket/file/path -> [..., "bucket", "file/path"]
    var groups = uri.match(/([^/:]+)\/(.*)/);
    var bucket = groups[1], path = groups[2];
    var url = 'https://www.googleapis.com/storage/v1/b/' + bucket + '/o/' +
      encodeURIComponent(path) + '?alt=media';
  } else {
    var url = uri;
  }
  var req = new XMLHttpRequest();
  req.open('GET', url);
  req.onload = function(resp) {
    callback(req);
  };
  req.onprogress = onprogress;
  req.send();
}

// One-time initialization of the whole page.
function load() {
  setOptionsFromURL();
  google.charts.load('current', {'packages': ['corechart', 'line']});
  google.charts.setOnLoadCallback(() => { google.charts.loaded = true });

  var setLoading = t => document.getElementById("loading-progress").innerText = t;
  var toMB = b => Math.round(b / 1024 / 1024 * 100) / 100;

  get('/k8s-gubernator/triage/failure_data.json',
    req => {
      setLoading(`parsing ${toMB(req.response.length)}MB.`);
      var data = JSON.parse(req.response);
      setTimeout(() => {
        builds = new Builds(data.builds);
        clusteredAll = new Clusters(data.clustered);
        rerender();
        if (window.location.hash) {
          renderUntilFound(window.location.hash.slice(1));
        }
      }, 0);
    },
    evt => {
      if (evt.type === "progress") {
        setLoading(`downloaded ${toMB(evt.loaded)}MB`);
      }
    }
  );

  document.addEventListener('click', clickHandler);
  document.addEventListener('scroll', scrollHandler);
}
