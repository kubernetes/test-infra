var assert = require('assert');
var build = require('./build');

describe('build', function() {
    describe('ansi_to_html', function() {
        function expect(name, before, after) {
            it(name, function() {
                assert.equal(build.ansi_to_html(before), after);
            });
        }
        expect('passes through unchanged text', 'something', 'something');
        expect('strips unknown codes', '\x1b[1;2;3fblah', 'blah');
        expect('bolds text', 'a \x1b[1mbold\x1b[0m plan', 'a <em>bold</em> plan');
        expect('handles color',
            '\x1b[31mred\x1b[0m \x1b[90mdog\x1b[0m', '<span class="ansi-1">red</span> <span class="ansi-8">dog</span>');
        expect('strips unpaired color commands',
            '\x1b[31mred \x1b[90mdog\x1b[0m', 'red <span class="ansi-8">dog</span>');
        expect('ignores unnecessary resets',
            '\x1b[0;37mgray\x1b[0m', '<span class="ansi-7">gray</span>')
        expect('handles color+bold',
            'foo \x1b[90m\x1b[1mdarkgray\x1b[0m', 'foo <em><span class="ansi-8">darkgray</span></em>');
    });
});
