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

describe('Clusters', () => {
    describe('refilter', () => {
        function expect(name, expected, clustered, opts) {
            it(name, function() {
                var c = new model.Clusters(clustered);
                assert.deepEqual(c.refilter(opts).data, expected);
            });
        }
        let ham = {text: 'ham', key: 'ham', id: '1234', tests: [
            {name: 'volume', jobs: [{name: 'cure', builds: [1, 2]}]},
        ]};
        let spam = {text: 'spam', key: 'spam', id: '5678', tests: [
            {name: 'networking', jobs: [{name: 'g', builds: [2]}]},
        ]};
        let pr = {text: 'bam', key: 'bam', id: '9abc', tests: [
            {name: 'new', jobs: [{name: 'pr:verify', builds: [3]}]},
        ]};
        let first = {text: 'afirst', key: 'afirst', id: 'def0', tests: [
            {name: 'something', jobs: [{name: 'firstjob', builds: [5, 6]}]},
        ]};
        expect('filters by text', [ham], [ham, spam], {reText: /ham/im, ci: true});
        expect('filters by test', [ham], [ham, spam], {reTest: /volume/im, ci: true});
        expect('filters by job', [ham], [ham, spam], {reJob: /cure/im, ci: true});
        expect('shows PRs when demanded', [pr], [ham, spam, pr], {pr: true});
        expect('hides PRs otherwise', [ham, spam], [ham, spam, pr], {ci: true});
        expect('can hide everything', [], [ham, spam, pr], {});
        expect('sorts results by build count', [ham, spam], [spam, ham], {ci: true, sort: 'total'});
        expect('sorts results by message', [first, ham, spam], [ham, spam, first], {ci: true, sort: 'message'});
    });
});
