(function () {
    'use strict';

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
    // JavaScript for some reason can map over arrays but nothing else.
    // Provide our own tools.
    function* map(iterable, fn) {
        for (const entry of iterable) {
            yield fn(entry);
        }
    }
    function reduce(iterable, fn, initialValue) {
        let accumulator = initialValue;
        for (const entry of iterable) {
            accumulator = fn(accumulator, entry);
        }
        return accumulator;
    }
    function* filter(iterable, fn) {
        for (const entry of iterable) {
            if (fn(entry)) {
                yield entry;
            }
        }
    }
    function* enumerate(iterable) {
        let i = 0;
        for (const entry of iterable) {
            yield [i++, entry];
        }
    }

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
    var __rest = (undefined && undefined.__rest) || function (s, e) {
        var t = {};
        for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p) && e.indexOf(p) < 0)
            t[p] = s[p];
        if (s != null && typeof Object.getOwnPropertySymbols === "function")
            for (var i = 0, p = Object.getOwnPropertySymbols(s); i < p.length; i++) if (e.indexOf(p[i]) < 0)
                t[p[i]] = s[p[i]];
        return t;
    };
    class FileCoverage {
        constructor(filename, fileNumber) {
            this.filename = filename;
            this.fileNumber = fileNumber;
            this.blocks = new Map();
        }
        addBlock(block) {
            const k = this.keyForBlock(block);
            const oldBlock = this.blocks.get(k);
            if (oldBlock) {
                oldBlock.hits += block.hits;
            }
            else {
                this.blocks.set(k, block);
            }
        }
        get totalStatements() {
            return reduce(this.blocks.values(), (acc, b) => acc + b.statements, 0);
        }
        get coveredStatements() {
            return reduce(this.blocks.values(), (acc, b) => acc + (b.hits > 0 ? b.statements : 0), 0);
        }
        keyForBlock(block) {
            return `${block.start.line}.${block.start.col},${block.end.line}.${block.end.col}`;
        }
    }
    class Coverage {
        constructor(mode, prefix = '') {
            this.mode = mode;
            this.prefix = prefix;
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
            for (const path of this.files.keys()) {
                // tslint:disable-next-line:prefer-const
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
                return this.prefix.substring(0, this.prefix.length - 1).split('/').pop() +
                    '/';
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
    function parseCoverage(content) {
        const lines = content.split('\n');
        const modeLine = lines.shift();
        const [modeLabel, mode] = modeLine.split(':').map((x) => x.trim());
        if (modeLabel !== 'mode') {
            throw new Error('Expected to start with mode line.');
        }
        // Well-formed coverage files are already sorted alphabetically, but Kubernetes'
        // `make test` produces ill-formed coverage files. This does actually matter, so
        // sort it ourselves.
        lines.sort((a, b) => {
            a = a.split(':', 2)[0];
            b = b.split(':', 2)[0];
            if (a < b) {
                return -1;
            }
            else if (a > b) {
                return 1;
            }
            else {
                return 0;
            }
        });
        const coverage = new Coverage(mode);
        let fileCounter = 0;
        for (const line of lines) {
            if (line === '') {
                continue;
            }
            const _a = parseLine(line), { filename } = _a, block = __rest(_a, ["filename"]);
            let file = coverage.getFile(filename);
            if (!file) {
                file = new FileCoverage(filename, fileCounter++);
                coverage.addFile(file);
            }
            file.addBlock(block);
        }
        return coverage;
    }
    function parseLine(line) {
        const [filename, block] = line.split(':');
        const [positions, statements, hits] = block.split(' ');
        const [start, end] = positions.split(',');
        const [startLine, startCol] = start.split('.').map(parseInt);
        const [endLine, endCol] = end.split('.').map(parseInt);
        return {
            end: {
                col: endCol,
                line: endLine,
            },
            filename,
            hits: Math.max(0, Number(hits)),
            start: {
                col: startCol,
                line: startLine,
            },
            statements: Number(statements),
        };
    }

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
    var __awaiter = (undefined && undefined.__awaiter) || function (thisArg, _arguments, P, generator) {
        return new (P || (P = Promise))(function (resolve, reject) {
            function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
            function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
            function step(result) { result.done ? resolve(result.value) : new P(function (resolve) { resolve(result.value); }).then(fulfilled, rejected); }
            step((generator = generator.apply(thisArg, _arguments || [])).next());
        });
    };
    let coverageFiles = [];
    let gPrefix = '';
    function filenameForDisplay(path) {
        const basename = path.split('/').pop();
        const withoutSuffix = basename.replace(/\.[^.]+$/, '');
        return withoutSuffix;
    }
    function loadEmbeddedProfiles() {
        return embeddedProfiles.map(({ path, content }) => ({
            coverage: parseCoverage(content),
            name: filenameForDisplay(path),
        }));
    }
    function init() {
        return __awaiter(this, void 0, void 0, function* () {
            if (location.hash.length > 1) {
                gPrefix = location.hash.substring(1);
            }
            coverageFiles = loadEmbeddedProfiles();
            google.charts.load('current', { packages: ['table'] });
            google.charts.setOnLoadCallback(drawTable);
        });
    }
    function updateBreadcrumb() {
        const parts = gPrefix.split('/');
        const parent = document.getElementById('breadcrumbs');
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
    function coveragesForPrefix(coverages, prefix) {
        const m = mergeMaps(map(coverages, (c) => c.getCoverageForPrefix(prefix).children));
        const keys = Array.from(m.keys());
        keys.sort();
        console.log(m);
        return map(keys, (k) => ({
            c: [{ v: k }].concat(m.get(k).map((x, i) => {
                if (!x) {
                    return { v: '' };
                }
                const next = m.get(k)[i + 1];
                const coverage = x.coveredStatements / x.totalStatements;
                let arrow = '';
                if (next) {
                    const nextCoverage = next.coveredStatements / next.totalStatements;
                    if (coverage > nextCoverage) {
                        arrow = '▲';
                    }
                    else if (coverage < nextCoverage) {
                        arrow = '▼';
                    }
                }
                const percentage = `${(coverage * 100).toFixed(1)}%`;
                return {
                    f: `<span class="arrow">${arrow}</span> ${percentage}`,
                    v: coverage,
                };
            })),
        }));
    }
    function mergeMaps(maps) {
        const result = new Map();
        for (const [i, m] of enumerate(maps)) {
            for (const [key, value] of m.entries()) {
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
    function drawTable() {
        const rows = Array.from(coveragesForPrefix(coverageFiles.map((x) => x.coverage), gPrefix));
        const cols = coverageFiles.map((x, i) => ({ id: `file-${i}`, label: x.name, type: 'number' }));
        const dataTable = new google.visualization.DataTable({
            cols: [
                { id: 'child', label: 'File', type: 'string' },
            ].concat(cols),
            rows,
        });
        const colourFormatter = new google.visualization.ColorFormat();
        colourFormatter.addGradientRange(0, 1.0001, '#FFFFFF', '#DD0000', '#00DD00');
        for (let i = 1; i < cols.length + 1; ++i) {
            colourFormatter.format(dataTable, i);
        }
        const table = new google.visualization.Table(document.getElementById('table'));
        table.draw(dataTable, { allowHtml: true });
        google.visualization.events.addListener(table, 'select', () => {
            const child = rows[table.getSelection()[0].row].c[0].v;
            if (child.endsWith('/')) {
                location.hash = gPrefix + child;
            }
        });
        updateBreadcrumb();
    }
    document.addEventListener('DOMContentLoaded', () => init());
    window.addEventListener('hashchange', () => {
        gPrefix = location.hash.substring(1);
        drawTable();
    });

}());
//# sourceMappingURL=zz.browser_bundle.es2015.js.map
