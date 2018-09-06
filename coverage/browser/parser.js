import {reduce, filter} from './utils.js';

function parseLine(line) {
  const [filename, block] = line.split(':');
  const [positions, statements, hits] = block.split(' ');
  const [start, end] = positions.split(',');
  const [startLine, startCol] = start.split('.').map(parseInt);
  const [endLine, endCol] = end.split('.').map(parseInt);
  return {
    filename,
    statements: parseInt(statements),
    hits: parseInt(hits),
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

export class FileCoverage {
  constructor(filename) {
    this.filename = filename;
    this.blocks = [];
  }

  addBlock(block) {
    this.blocks.push(block);
  }

  get totalStatements() {
    return this.blocks.reduce((acc, b) => acc + b.statements, 0);
  }

  get coveredStatements() {
    return this.blocks.reduce((acc, b) => acc + (b.hits > 0 ? b.statements : 0), 0);
  }
}

export class Coverage {
  constructor(mode, prefix) {
    this.mode = mode;
    this.prefix = prefix || '';
    this.files = new Map();
  }

  addFile(file) {
    this.files.set(file.filename, file);
  }

  getFile(name) {
    return this.files.get(name);
  }

  getFilesWithPrefix(prefix) {
    return new Map(filter(this.files.entries(), ([k]) => k.startsWith(this.prefix + prefix)));
  }

  getCoverageForPrefix(prefix) {
    const subCoverage = new Coverage(this.mode, this.prefix + prefix);
    for (const [filename, file] of this.files) {
      if (filename.startsWith(this.prefix + prefix)) {
        subCoverage.addFile(file);
      }
    }
    return subCoverage;
  }

  get children() {
    const children = new Map();
    for (let path of this.files.keys()) {
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

  get basename() {
    if (this.prefix.endsWith('/')) {
      return this.prefix.substring(0, this.prefix.length - 1).split('/').pop() + '/';
    }
    return this.prefix.split('/').pop();
  }

  get totalStatements() {
    return reduce(this.files.values(), (acc, f) => acc + f.totalStatements, 0);
  }

  get coveredStatements() {
    return reduce(this.files.values(), (acc, f) => acc + f.coveredStatements, 0);
  }

  get totalFiles() {
    return this.files.size;
  }

  get coveredFiles() {
    return reduce(this.files.values(), (acc, f) => acc + (f.coveredStatements > 0 ? 1 : 0), 0);
  }
}

export function parseCoverage(content) {
  const lines = content.split("\n");
  const modeLine = lines.shift();
  const [modeLabel, mode] = modeLine.split(':').map(x => x.trim());
  if (modeLabel !== "mode") {
    throw new Error("Expected to start with mode line.");
  }

  const coverage = new Coverage(mode);
  for (const line of lines) {
    if (line === "") {
      continue;
    }
    const {filename, ...block} = parseLine(line);
    let file = coverage.getFile(filename);
    if (!file) {
      file = new FileCoverage(filename);
      coverage.addFile(file);
    }
    file.addBlock(block);
  }

  return coverage;
}
