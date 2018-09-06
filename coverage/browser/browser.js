/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import { parseCoverage } from './parser.js';
import { map, enumerate } from './utils.js';

let coverageFiles = [];
let prefix = 'k8s.io/kubernetes/';

function filterCoverage(coverage) {
  const toRemove = [];
  for (let file of coverage.files.keys()) {
    if (file.match(/zz_generated|third_party\/|cmd\/|cloudprovider\/providers\/|alpha|beta/)) {
      toRemove.push(file);
    }
  }
  console.log(`Filtering out ${toRemove.length} files.`);
  for (let file of toRemove) {
    coverage.files.delete(file);
  }
  return coverage;
}

async function loadProfile(path) {
  const response = await fetch(path, {credentials: 'include'});
  const content = await response.text();
  return filterCoverage(parseCoverage(content));
}

async function init() {
  if (location.hash.length > 1) {
    prefix = location.hash.substring(1);
  }
  // TODO: this path shouldn't be hardcoded.
  coverageFiles = [await loadProfile("profiles/coverage-1.12.cov"), await loadProfile("profiles/coverage-1.11.cov")];
  google.charts.load('current', {'packages': ['table']});
  google.charts.setOnLoadCallback(drawTable);
}

function updateBreadcrumb() {
  let parts = prefix.split('/');
  let parent = document.getElementById('breadcrumbs');
  parent.innerHTML = '';
  let prefixSoFar = '';
  for (let part of parts) {
    if (!part) {
      continue;
    }
    prefixSoFar += part + '/';
    let node = document.createElement('a');
    node.href = `#${prefixSoFar}`;
    node.innerText = part;
    parent.appendChild(node);
    parent.appendChild(document.createTextNode('/'));
  }

}

function coveragesForPrefix(coverages, prefix) {
  const m = mergeMaps(map(coverages, (c) => c.getCoverageForPrefix(prefix).children));
  const keys = Array.from(m.keys());
  keys.sort();
  console.log(m);
  return map(keys, (k) => ({c: [{v: k}].concat(m.get(k).map((x, i) => {
    if (!x) {
      return {v: ''};
    }
    const next = m.get(k)[i+1];
    const coverage = x.coveredStatements / x.totalStatements;
    const nextCoverage = next ? next.coveredStatements / next.totalStatements : null;
    let arrow = '';
    if (next) {
      if (coverage > nextCoverage) {
        arrow = '▲';
      } else if (coverage < nextCoverage) {
        arrow = '▼';
      }
    }
    return {
      v: coverage,
      f: `<span style="float: left; margin-right: 5px;">${arrow}</span> ${(coverage * 100).toFixed(1)}%`
    }
  }))}));
}

function mergeMaps(maps) {
  const result = new Map();
  for (let [i, map] of enumerate(maps)) {
    for (let [key, value] of map.entries()) {
      if (!result.has(key)) {
        result.set(key, Array(i).fill(null));
      }
      result.get(key).push(value);
    }
    for (let entry of result.values()) {
      if (entry.length === i) {
        entry.push(null);
      }
    }
  }
  return result;
}

function drawTable() {
  // const rows = Array.from(map(coverage.getCoverageForPrefix(prefix).children.values(), x => ({c: [{v: x.basename},
  //     {v: x.coveredStatements / x.totalStatements, f: `${(x.coveredStatements / x.totalStatements * 100).toFixed(1)}%`},
  //     {v: x.coveredFiles / x.totalFiles, f: `${(x.coveredFiles / x.totalFiles * 100).toFixed(1)}%`},
  //   ]})));
  const rows = Array.from(coveragesForPrefix(coverageFiles, prefix));
  const dataTable = new google.visualization.DataTable({
    cols: [{id: 'child', label: 'File', type: 'string'},
      {id: 'statement-coverage', label: '1.12', type: 'number'},
      {id: 'statement-coverage', label: '1.11', type: 'number'},
    ],
    rows
  });

  const colourFormatter = new google.visualization.ColorFormat();
  colourFormatter.addGradientRange(0, 1.0001, '#FFFFFF', '#DD0000', '#00DD00');
  // colourFormatter.addGradientRange(0.5, 1.0001, '#FFFFFF', '#00DD00', '#00DD00');
  for (let i = 1; i < rows[0].c.length; ++i) {
    colourFormatter.format(dataTable, i);
  }

  const table = new google.visualization.Table(document.getElementById('table'));
  table.draw(dataTable, {allowHtml: true});

  google.visualization.events.addListener(table, 'select', () => {
    const child = rows[table.getSelection()[0].row].c[0].v;
    if (child.endsWith('/')) {
      location.hash = prefix + child;
    } else {
      // TODO: this shouldn't be hardcoded.
      location.href = 'profiles/everything-diff.html#file' + coverage.getFile(prefix + child).fileNumber;
    }
  });
  updateBreadcrumb();
}

document.addEventListener('DOMContentLoaded', () => init());
window.addEventListener('hashchange', () => {
  prefix = location.hash.substring(1);
  drawTable();
});