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

import {Coverage, parseCoverage} from './parser';
import {enumerate, map} from './utils';

declare const embeddedProfiles: Array<{path: string, content: string}>;

let coverageFiles: Array<{name: string, coverage: Coverage}> = [];
let prefix = 'k8s.io/kubernetes/';

function filenameForDisplay(path: string): string {
  const basename = path.split('/').pop()!;
  const withoutSuffix = basename.replace(/\.[^.]+$/, '');
  return withoutSuffix;
}

function loadEmbeddedProfiles(): Array<{name: string, coverage: Coverage}> {
  return embeddedProfiles.map(({path, content}) => ({
                                name: filenameForDisplay(path),
                                coverage: parseCoverage(content),
                              }));
}

async function loadProfile(path: string): Promise<Coverage> {
  const response = await fetch(path, {credentials: 'include'});
  const content = await response.text();
  return parseCoverage(content);
}

async function init(): Promise<void> {
  if (location.hash.length > 1) {
    prefix = location.hash.substring(1);
  }

  coverageFiles = loadEmbeddedProfiles();
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
      keys,
      (k) => ({
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
              const percentage = `${(coverage * 100).toFixed(1)}%`;
              return {
                v: coverage,
                    f: `<span class="arrow">${arrow}</span> ${percentage}`,
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
  const rows = Array.from(
      coveragesForPrefix(coverageFiles.map((x) => x.coverage), prefix));
  const cols = coverageFiles.map(
      (x, i) => ({id: `file-${i}`, label: x.name, type: 'number'}));
  const dataTable = new google.visualization.DataTable({
    cols: [
      {id: 'child', label: 'File', type: 'string'},
    ].concat(cols),
    rows
  });

  const colourFormatter = new google.visualization.ColorFormat();
  colourFormatter.addGradientRange(0, 1.0001, '#FFFFFF', '#DD0000', '#00DD00');
  for (let i = 1; i < cols.length + 1; ++i) {
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
