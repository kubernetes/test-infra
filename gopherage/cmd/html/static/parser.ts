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

import {filter, reduce} from './utils';

export interface Pos {
  line: number;
  col: number;
}

export interface Block {
  statements: number;
  hits: number;
  start: Pos;
  end: Pos;
}


export class FileCoverage {
  blocks: Block[] = [];

  constructor(readonly filename: string, readonly fileNumber: number) {}

  addBlock(block: Block) {
    this.blocks.push(block);
  }

  get totalStatements(): number {
    return this.blocks.reduce((acc, b) => acc + b.statements, 0);
  }

  get coveredStatements(): number {
    return this.blocks.reduce(
        (acc, b) => acc + (b.hits > 0 ? b.statements : 0), 0);
  }
}

export class Coverage {
  files = new Map<string, FileCoverage>();

  constructor(readonly mode: string, readonly prefix = '') {}

  addFile(file: FileCoverage): void {
    this.files.set(file.filename, file);
  }

  getFile(name: string): FileCoverage|undefined {
    return this.files.get(name);
  }

  getFilesWithPrefix(prefix: string): Map<string, FileCoverage> {
    return new Map(filter(
        this.files.entries(), ([k]) => k.startsWith(this.prefix + prefix)));
  }

  getCoverageForPrefix(prefix: string): Coverage {
    const subCoverage = new Coverage(this.mode, this.prefix + prefix);
    for (const [filename, file] of this.files) {
      if (filename.startsWith(this.prefix + prefix)) {
        subCoverage.addFile(file);
      }
    }
    return subCoverage;
  }

  get children(): Map<string, Coverage> {
    const children = new Map();
    for (const path of this.files.keys()) {
      let [dir, rest] = path.substr(this.prefix.length).split('/', 2);
      if (!children.has(dir)) {
        if (rest) {
          dir += '/';
        }
        children.set(dir, this.getCoverageForPrefix(dir));
      }
    }
    return children;
  }

  get basename(): string {
    if (this.prefix.endsWith('/')) {
      return this.prefix.substring(0, this.prefix.length - 1).split('/').pop() +
          '/';
    }
    return this.prefix.split('/').pop()!;
  }

  get totalStatements(): number {
    return reduce(this.files.values(), (acc, f) => acc + f.totalStatements, 0);
  }

  get coveredStatements(): number {
    return reduce(
        this.files.values(), (acc, f) => acc + f.coveredStatements, 0);
  }

  get totalFiles(): number {
    return this.files.size;
  }

  get coveredFiles(): number {
    return reduce(
        this.files.values(),
        (acc, f) => acc + (f.coveredStatements > 0 ? 1 : 0), 0);
  }
}

export function parseCoverage(content: string): Coverage {
  const lines = content.split('\n');
  const modeLine = lines.shift()!;
  const [modeLabel, mode] = modeLine.split(':').map(x => x.trim());
  if (modeLabel !== 'mode') {
    throw new Error('Expected to start with mode line.');
  }

  const coverage = new Coverage(mode);
  let fileCounter = 0;
  for (const line of lines) {
    if (line === '') {
      continue;
    }
    const {filename, ...block} = parseLine(line);
    let file = coverage.getFile(filename);
    if (!file) {
      file = new FileCoverage(filename, fileCounter++);
      coverage.addFile(file);
    }
    file.addBlock(block);
  }

  return coverage;
}

function parseLine(line: string): Block&{filename: string} {
  const [filename, block] = line.split(':');
  const [positions, statements, hits] = block.split(' ');
  const [start, end] = positions.split(',');
  const [startLine, startCol] = start.split('.').map(parseInt);
  const [endLine, endCol] = end.split('.').map(parseInt);
  return {
    filename,
    statements: Number(statements),
    hits: Math.max(0, Number(hits)),
    start: {
      line: startLine,
      col: startCol,
    },
    end: {
      line: endLine,
      col: endCol,
    },
  };
}
