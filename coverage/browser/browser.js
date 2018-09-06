import { parseCoverage } from './parser.js';
import { map } from './utils.js';

let coverage = null;
let prefix = 'k8s.io/kubernetes/';

async function loadProfile(path) {
  const response = await fetch(path, {credentials: 'include'});
  const content = await response.text();
  return parseCoverage(content);
}

async function init() {
  if (location.hash.length > 1) {
    prefix = location.hash.substring(1);
  }
  coverage = await loadProfile("profiles/everything-diff.cov");
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

function drawTable() {
  const rows = Array.from(map(coverage.getCoverageForPrefix(prefix).children.values(), x => ({c: [{v: x.basename},
      {v: x.coveredStatements / x.totalStatements, f: `${(x.coveredStatements / x.totalStatements * 100).toFixed(1)}%`},
      {v: x.coveredFiles / x.totalFiles, f: `${(x.coveredFiles / x.totalFiles * 100).toFixed(1)}%`},
    ]})));
  const dataTable = new google.visualization.DataTable({
    cols: [{id: 'child', label: 'File', type: 'string'},
      {id: 'statement-coverage', label: 'Statement coverage', type: 'number'},
      {id: 'file-coverage', label: 'File coverage', type: 'number'}],
    rows
  });

  const colourFormatter = new google.visualization.ColorFormat();
  colourFormatter.addGradientRange(0, 1.0001, '#FFFFFF', '#DD0000', '#00DD00');
  colourFormatter.format(dataTable, 1);
  colourFormatter.format(dataTable, 2);

  const table = new google.visualization.Table(document.getElementById('table'));
  table.draw(dataTable, {allowHtml: true});

  google.visualization.events.addListener(table, 'select', () => {
    const child = rows[table.getSelection()[0].row].c[0].v;
    if (child.endsWith('/')) {
      location.hash = prefix + child;
    }
  });
  updateBreadcrumb();
}

document.addEventListener('DOMContentLoaded', () => init());
window.addEventListener('hashchange', () => {
  prefix = location.hash.substring(1);
  drawTable();
});