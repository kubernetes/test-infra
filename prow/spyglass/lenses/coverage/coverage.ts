/*
Copyright 2019 The Kubernetes Authors.

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

import Color from "color";
import {Coverage, parseCoverage} from 'io_k8s_test_infra/gopherage/cmd/html/static/parser';
import {inflate} from "pako/lib/inflate";

declare const COVERAGE_FILE: string;

const NO_COVERAGE = Color('#FF0000');
const FULL_COVERAGE = Color('#00FF00');

// Inspired by https://dl.acm.org/citation.cfm?id=949654
function renderChildren(parent: Node, coverage: Coverage, horizontal: boolean): void {
  let offset = 0;
  for (const child of coverage.children.values()) {
    const node = document.createElement('div');
    parent.appendChild(node);
    const percentage = child.totalStatements / coverage.totalStatements * 100;
    node.style.position = 'absolute';
    if (horizontal) {
      node.style.width = `${percentage}%`;
      node.style.height = '100%';
      node.style.top = '0';
      node.style.left = `${offset}%`;
    } else {
      node.style.width = '100%';
      node.style.height = `${percentage}%`;
      node.style.top = `${offset}%`;
      node.style.left = `0`;
    }
    offset += percentage;
    if (child.totalFiles === 1) {
      node.classList.add('leaf');
      const [filename, file] = child.files.entries().next().value;
      node.title = `${filename}: ${(file.coveredStatements / file.totalStatements * 100).toFixed(0)}%`;
      node.style.backgroundColor = NO_COVERAGE.mix(FULL_COVERAGE, file.coveredStatements / file.totalStatements).hex();
      // Not having a border looks weird, but using a constant colour causes tiny boxes
      // to consist entirely of that colour. By using a border colour based on the
      // box colour, we still show some information.
      node.style.borderColor = Color(node.style.backgroundColor).darken(0.3).hex();
    } else {
      renderChildren(node, child, !horizontal);
    }
  }
}

window.onload = () => {
  // Because the coverage files are a) huge, and b) compress excellently, we send it as
  // gzipped base64. This is faster unless your internet connection is faster than
  // about 300 Mb/s.
  const content = inflate(atob(COVERAGE_FILE), {to: 'string'});
  const coverage = parseCoverage(content);
  document.getElementById('statement-coverage')!.innerText = `${(coverage.coveredStatements / coverage.totalStatements * 100).toFixed(0)}% (${coverage.coveredStatements.toLocaleString()} of ${coverage.totalStatements.toLocaleString()} statements)`;
  document.getElementById('file-coverage')!.innerText = `${(coverage.coveredFiles / coverage.totalFiles * 100).toFixed(0)}% (${coverage.coveredFiles.toLocaleString()} of ${coverage.totalFiles.toLocaleString()} files)`;
  renderChildren(document.getElementById('treemap')!, coverage, true);
};
