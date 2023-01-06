const assert = require('assert');
const model = require('./model');
const render = require('./render');

describe('makeBuckets', () => {
    function expect(name, expected, ...args) {
        it(name, function() {
            assert.deepEqual(render.makeBuckets(...args), expected);
        });
    }
    expect('makes a histogram',           [[0, 3], [4, 2]],     [0, 1, 2, 4, 5], 4, 0, 4)
    expect('expands to fill range',       [[0, 0], [4, 0], [8, 0], [12, 0]], [], 4, 0, 12);
    expect('shifts start to match width', [[0, 1], [4, 1]],     [2, 6], 4, 2, 6);
});

describe('sparkLinePath', () => {
    function expect(name, expected, ...args) {
        it(name, function() {
            assert.deepEqual(render.sparkLinePath(...args), expected);
        });
    }
    expect('draws a zero graph', 'M0,9h5', [0,0,0,0,0], 1, 9);
    expect('draws a spikey graph', 'M0,9h1V0h1V9h1', [0,1,0], 1, 9);
    expect('combines adjacents spans', 'M0,9h1V4h2V0h1V9h1', [0,1,1,2,0], 1, 9);
    expect('handles scaling', 'M0,8h0V7h2V6h1V4h1V0h1V8', [2,4,8,16,32], 1, 8);
})

describe('Clusters', () => {
    describe('refilter', () => {
        function expect(name, expected, clustered, opts) {
            it(name, function() {
                var c = new model.Clusters(clustered);
                assert.deepEqual(c.refilter(opts).data, expected);
            });
        }
        let ham = {text: 'ham', key: 'ham', id: '1234', owner: 'node', tests: [
            {name: 'volume', jobs: [{name: 'cure', builds: [1, 2]}]},
        ]};
        let spam = {text: 'spam', key: 'spam', id: '5678', owner: 'ui', tests: [
            {name: 'networking', jobs: [{name: 'gcure', builds: [2]}]},
        ]};
        let pr = {text: 'bam', key: 'bam', id: '9abc', tests: [
            {name: 'new', jobs: [{name: 'pr:verify', builds: [3]}]},
        ]};
        let first = {text: 'afirst', key: 'afirst', id: 'def0', tests: [
            {name: 'something', jobs: [{name: 'firstjob', builds: [5, 6]}]},
        ]};
        expect('filters by text', [ham, spam], [ham, spam, pr], {reText: /am/im, reXText: /b/im, ci: true});
        expect('filters by test', [spam], [ham, spam, first], {reTest: /ing/im, reXTest: /some/im, ci: true});
        expect('filters by job', [ham], [ham, spam], {reJob: /cure/im, reXJob: /g/im, ci: true});
        expect('filters by sig', [ham], [ham, spam], {sig: ['node'], ci: true});
        expect('shows PRs when demanded', [pr], [ham, spam, pr], {pr: true});
        expect('hides PRs otherwise', [ham, spam], [ham, spam, pr], {ci: true});
        expect('can hide everything', [], [ham, spam, pr], {});
        expect('sorts results by build count', [ham, spam], [spam, ham], {ci: true, sort: 'total'});
        expect('sorts results by message', [first, ham, spam], [ham, spam, first], {ci: true, sort: 'message'});
    });
});
