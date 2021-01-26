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
    if (el.type === "select-one" || el.type === "date") return el.value;
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

  function readSigs() {
    var ret = [];
    for (let el of document.getElementById("btn-sig-group").children) {
      if (el.classList.contains('active')) {
        ret.push(el.textContent);
      }
    }
    return ret;
  }

  var opts = {
    date: read('date'),
    ci: read('job-ci'),
    pr: read('job-pr'),
    reText: read('filter-include-text'),
    reJob: read('filter-include-job'),
    reTest: read('filter-include-test'),
    reXText: read('filter-exclude-text'),
    reXJob: read('filter-exclude-job'),
    reXTest: read('filter-exclude-test'),
    showNormalize: read('show-normalize'),
    sort: read('sort'),
    sig: readSigs(),
  };

  var url = '';
  if (opts.date) url += '&date=' + opts.date;
  if (!opts.ci) url += '&ci=0';
  if (opts.pr) url += '&pr=1';
  if (opts.sig.length) url += '&sig=' + opts.sig.join(',');
  for (var name of ["text", "job", "test", "xtext", "xjob", "xtest"]) {
    var re = (name[0] == 'x') ?
      opts['reX' + name[1].toUpperCase() + name.slice(2)] :
      opts['re'  + name[0].toUpperCase() + name.slice(1)];
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
    else el.value = value;
  }

  function writeSigs(sigs) {
    for (let sig of (sigs || '').split(',')) {
      var el = document.getElementById('btn-sig-' + sig);
      if (el) {
        el.classList.add('active');
      }
    }
  }

  write('date', qs.date);
  write('job-ci', qs.ci);
  write('job-pr', qs.pr);
  write('filter-include-text', qs.text);
  write('filter-include-job', qs.job);
  write('filter-include-test', qs.test);
  write('filter-exclude-text', qs.xtext);
  write('filter-exclude-job', qs.xjob);
  write('filter-exclude-test', qs.xtest);
  writeSigs(qs.sig);
}

// Render up to `count` clusters, with `start` being the first for consideration.
function renderSubset(start, count) {
  var top = document.getElementById('clusters');
  var n = 0;
  var shown = 0;
  for (let c of clustered.data) {
    if (n++ < start) continue;
    shown += renderCluster(top, c);
    lastClusterRendered = n;
    if (shown >= count) break;
  }
}

function setElementVisibility(id, visible) {
  document.getElementById(id).style.display = visible ? null : 'none';
}

// Clear the page and reinitialize the renderer and filtering. Render a few failures.
function rerender(maxCount) {
  if (!clusteredAll) return;

  console.log('rerender!');

  setElementVisibility('load-status', false);
  setElementVisibility('clusters', true);

  options = readOptions();
  clustered = clusteredAll.refilter(options);

  var top = document.getElementById('clusters');
  var summary = document.getElementById('summary');
  top.removeChildren();
  summary.removeChildren();

  var summaryText = `
            ${clustered.length} clusters of ${clustered.sum} failures`;

  if (clustered.sumRecent > 0) {
    summaryText += ` (${clustered.sumRecent} in last day)`;
  }

  summaryText += ` out of ${builds.runCount} builds from ${builds.getStartTime()} to ${builds.getEndTime()}.`

  if (clusteredAll.clusterId) {
    // Render just the cluster with the given key.
    // Show an error message if no live cluster with that id is found.
    summaryText = '';
    var keyId = clusteredAll.clusterId;

    addElement(top, 'h3', null, [createElement('a', {href: ''}, 'View all clusters')]);

    if (!clustered.byId[keyId]) {
      var summary = document.getElementById('summary');
      summaryText = `Cluster ${keyId} not found in the last week of data.`
    }
  }

  if (maxCount !== 0) {
    summary.innerText = summaryText;

    if (clustered.length > 0 && !clusteredAll.clusterId) {
      let graph = addElement(summary, 'div');
      renderGraph(graph, clustered.allBuilds());
    }

    renderSubset(0, maxCount || 10);

    // draw graphs after the current render cycle, to reduce perceived latency.
    setTimeout(drawVisibleGraphs, 0);
  }
}

function toggle(target) {
  if (target.matches('button.toggle')) {
    target.classList.toggle("active");
    // rerender after repainting the clicked button, to improve responsiveness.
    setTimeout(rerender, 0);
  } else if (target.matches('span.owner')) {
    document.getElementById('btn-sig-' + target.textContent).click();
  } else if (target.matches('.clearoptions')) {
    document.location = document.location.pathname;
  } else {
    return false;
  }
  return true;
}

function renderOnly(keyId) {
  var el = null;
  rerender(0);

  var top = document.getElementById('clusters');
  top.removeChildren();

  renderSubset(0, 1);

  // expand the graph for the selected failure.
  setTimeout(drawVisibleGraphs, 0);
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
  if (expand(target) || toggle(target)) {
    evt.preventDefault();
    return true;
  }
  return false;
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

function getData() {
  var clusterId = null;
  if (/^#[a-f0-9]{20}$/.test(window.location.hash)) {
    clusterId = window.location.hash.slice(1);
    // Hide filtering options, since this page has only a single cluster.
    setElementVisibility('multiple-options', false);
    setElementVisibility('btn-sig-group', false);
  }

  var url = '/k8s-gubernator/triage/';
  if (document.location.host == 'storage.googleapis.com' && document.location.pathname.endsWith('index.html')) {
    // Use the bucket name where available
    var pathname = document.location.pathname;
    url = pathname.substring(0, pathname.lastIndexOf('/')+1);
  }
  var date = document.getElementById('date');
  if (date && date.value) {
    url += 'history/' + date.value.replace(/-/g, '') + '.json';
  } else if (clusterId) {
    url += 'slices/failure_data_' + clusterId.slice(0, 2) + '.json';
  } else {
    url += 'failure_data.json'
  }

  setElementVisibility('load-status', true);
  setElementVisibility('clusters', false);
  var setLoading = t => document.getElementById("loading-progress").innerText = t;
  var toMB = b => Math.round(b / 1024 / 1024 * 100) / 100;

  get(url,
    req => {
      if (req.status >= 300) {
        if (req.status == 401) {
          setLoading(`error ${req.status}: missing data (bad date?): ${req.response}`);
        } else {
          setLoading(`error ${req.status}: ${req.response}`)
        }
        return;
      }
      setLoading(`parsing ${toMB(req.response.length)}MB.`);
      setTimeout(() => {
        var data = JSON.parse(req.response);
        builds = new Builds(data.builds);
        if (clusterId) {
          // rendering just one cluster, filter here.
          for (let c of data.clustered) {
            if (c.id == clusterId) {
              data.clustered = [c];
              break;
            }
          }
        }
        clusteredAll = new Clusters(data.clustered, clusterId);
        rerender();
      }, 0);
    },
    evt => {
      if (evt.type === "progress") {
        setLoading(`downloaded ${toMB(evt.loaded)}MB`);
      }
    }
  );
}

// One-time initialization of the whole page.
function load() {
  setOptionsFromURL();

  getData();

  google.charts.load('current', {'packages': ['corechart', 'line']});
  google.charts.setOnLoadCallback(() => { google.charts.loaded = true });

  document.addEventListener('click', clickHandler, false);
  document.addEventListener('scroll', scrollHandler);

  document.getElementById('date').max = new Date().toISOString().slice(0, 10);
}
