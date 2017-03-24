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
        console.log("DE1 " + assert.deepEqual);
        function expect(name, expected, clustered, opts) {
            it(name, function() {
                var c = new model.Clusters(clustered);
                assert.deepEqual(c.refilter(opts).data, expected);
            });
        }
        let ham = ['ham', '', 'ham', [
            ['volume', [['cure', [1, 2]]]],
        ]];
        let spam = ['spam', '', 'spam', [
            ['networking', [['g', [2]]]]
        ]];
        let pr = ['bam', '', 'bam', [
            ['new', [['pr:verify', [3]]]],
        ]];
        let first = ['afirst', '', 'afirst', [
            ['something', [['firstjob', [5, 6]]]],
        ]];
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
