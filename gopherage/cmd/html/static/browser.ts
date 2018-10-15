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

import {Coverage, FileCoverage, parseCoverage} from './parser';
import {enumerate, map} from './utils';

declare const coverage: string;

let coverageFiles: Coverage[] = [];
let prefix = 'k8s.io/kubernetes/';

function filterCoverage(coverage: Coverage): Coverage {
  const toRemove = [];
  for (const file of coverage.files.keys()) {
    if (file.match(
            /zz_generated|third_party\/|cmd\/|cloudprovider\/providers\/|alpha|beta/)) {
      toRemove.push(file);
    }
  }
  console.log(`Filtering out ${toRemove.length} files.`);
  for (const file of toRemove) {
    coverage.files.delete(file);
  }
  return coverage;
}

function loadEmbeddedProfile(): Coverage {
  return filterCoverage(parseCoverage(coverage));
}

async function loadProfile(path: string): Promise<Coverage> {
  const response = await fetch(path, {credentials: 'include'});
  const content = await response.text();
  return filterCoverage(parseCoverage(content));
}

async function init(): Promise<void> {
  if (location.hash.length > 1) {
    prefix = location.hash.substring(1);
  }
  // TODO: this path shouldn't be hardcoded.
  coverageFiles = [loadEmbeddedProfile()];
  google.charts.load('current', {'packages': ['table']});
  google.charts.setOnLoadCallback(drawTable);
}

function updateBreadcrumb(): void {
  const parts = prefix.split('/');
  const parent = document.getElementById('breadcrumbs')!;
  parent.innerHTML = '';
  let prefixSoFar = '';
  for (const part of parts) {
    if (!part) {
      continue;
    }
    prefixSoFar += part + '/';
    const node = document.createElement('a');
    node.href = `#${prefixSoFar}`;
    node.innerText = part;
    parent.appendChild(node);
    parent.appendChild(document.createTextNode('/'));
  }
}

function coveragesForPrefix(coverages: Coverage[], prefix: string):
    Iterable<{c: Array<{v: number | string, f?: string}>}> {
  const m =
      mergeMaps(map(coverages, (c) => c.getCoverageForPrefix(prefix).children));
  const keys = Array.from(m.keys());
  keys.sort();
  console.log(m);
  return map(
      keys, (k) => ({
              c: [({v: k} as {v: number | string, f?: string})].concat(
                  m.get(k)!.map((x, i) => {
                    if (!x) {
                      return {v: ''};
                    }
                    const next = m.get(k)![i + 1];
                    const coverage = x.coveredStatements / x.totalStatements;
                    let arrow = '';
                    if (next) {
                      const nextCoverage =
                          next.coveredStatements / next.totalStatements;
                      if (coverage > nextCoverage) {
                        arrow = '▲';
                      } else if (coverage < nextCoverage) {
                        arrow = '▼';
                      }
                    }
                    return {
                      v: coverage,
                          f: `<span style="float: left; margin-right: 5px;">${
                              arrow}</span> ${(coverage * 100).toFixed(1)}%`
                    }
                  }))
            }));
}

function mergeMaps<T, U>(maps: Iterable<Map<T, U>>): Map<T, U[]> {
  const result = new Map();
  for (const [i, map] of enumerate(maps)) {
    for (const [key, value] of map.entries()) {
      if (!result.has(key)) {
        result.set(key, Array(i).fill(null));
      }
      result.get(key).push(value);
    }
    for (const entry of result.values()) {
      if (entry.length === i) {
        entry.push(null);
      }
    }
  }
  return result;
}

function drawTable(): void {
  const rows = Array.from(coveragesForPrefix(coverageFiles, prefix));
  const dataTable = new google.visualization.DataTable({
    cols: [
      {id: 'child', label: 'File', type: 'string'},
      {id: 'statement-coverage', label: 'Coverage', type: 'number'},
    ],
    rows
  });

  const colourFormatter = new google.visualization.ColorFormat();
  colourFormatter.addGradientRange(0, 1.0001, '#FFFFFF', '#DD0000', '#00DD00');
  for (let i = 1; i < rows[0].c.length; ++i) {
    colourFormatter.format(dataTable, i);
  }

  const table =
      new google.visualization.Table(document.getElementById('table')!);
  table.draw(dataTable, {allowHtml: true});

  google.visualization.events.addListener(table, 'select', () => {
    const child = rows[table.getSelection()[0].row!].c[0].v as string;
    if (child.endsWith('/')) {
      location.hash = prefix + child;
    } else {
      // TODO: this shouldn't be hardcoded.
      // location.href = 'profiles/everything-diff.html#file' +
      //     coverage.getFile(prefix + child).fileNumber;
    }
  });
  updateBreadcrumb();
}

document.addEventListener('DOMContentLoaded', () => init());
window.addEventListener('hashchange', () => {
  prefix = location.hash.substring(1);
  drawTable();
});
