// Given a DOM node, attempt to select it.
function select(node) {
	var sel = window.getSelection();
	if (sel.toString() !== "") {
		// User is already trying to do a drag-selection, don't prevent it.
		return;
	}
	// Works in Chrome/Safari/FF/IE10+
	var range = document.createRange();
	range.selectNode(node);
	sel.removeAllRanges();
	sel.addRange(range);
}

// Rewrite timestamps to respect the current locale.
function fix_timestamps() {
	function replace(className, fmt) {
		var tz = moment.tz.guess();
		var els = document.getElementsByClassName(className);
		for (var i = 0; i < els.length; i++) {
			var el = els[i];
			var epoch = el.getAttribute('data-epoch');
			if (epoch) {
				var time = moment(1000 * epoch).tz(tz);
				if (typeof fmt === 'function') {
					el.innerText = fmt(time);
				} else {
					el.innerText = time.format(fmt);
				}
			}
		}
	}
	replace('timestamp', 'YYYY-MM-DD HH:mm z')
	replace('shorttimestamp', 'MM-DD HH:mm')
	replace('humantimestamp', function(t) {
		var fmt = 'MMM D, Y';
		if (t.isAfter(moment().startOf('day'))) {
			fmt = 'h:mm A';
		} else if (t.isAfter(moment().startOf('year'))) {
			fmt = 'MMM D';
		}
		return t.format(fmt);
	})
}

var get_cache = {};

// Download a file from GCS or elsewhere, and run "callback" with its contents.
function get(uri, callback) {
	if (get_cache[uri]) {
		callback(get_cache[uri]);
		return;
	}
	if (uri[0] === '/') {
		// Matches gs://bucket/file/path -> [..., "bucket", "file/path"]
		//         /bucket/file/path -> [..., "bucket", "file/path"]
		var groups = uri.match(/([^/:]+)\/(.*)/);
		var bucket = groups[1], path = groups[2];
		var url = 'https://www.googleapis.com/storage/v1/b/' + bucket + '/o/' +
			encodeURIComponent(path) + '?alt=media';
	} else {
		var url = uri;
	}
	var req = new XMLHttpRequest();
	req.open('GET', url);
	req.onload = function(resp) {
		get_cache[uri] = {status: req.status, response: req.response};
		callback(req);
	}
	req.send();
}

function expand_lines(els, data) {
	var lines = data.split('\n');
	var parent = els[0].parentElement;
	for (var i = 0; i < els.length; i++) {
		var el = els[i];
		var range = el.dataset['range'].split('-');
		var chunk = lines.slice(range[0], range[1]);
		var chunk = chunk.join('\n');
		if (el.previousSibling) {
			el.previousSibling.appendData(chunk);
			el.remove();
		} else if (el.nextSibling) {
			el.nextSibling.data = chunk + el.nextSibling.data;
			el.remove();
		}
	}
	parent.normalize();  // merge adjacent text nodes
	fix_escape_codes();  // colorize new segments
}

function expand_skipped(els) {
	var src = els[0].parentElement.dataset['src'];
	get(src, function(req) {
		if (req.status == 401) {  // unauthorized
			// try proxying through Gubernator
			var proxyUrl = document.location.origin + '/gcsproxy?path=' + escape(src);
			document.getElementById('rawloglink').href = proxyUrl
			get(proxyUrl, function (req) { expand_lines(els, req.response); })
		} else {
			expand_lines(els, req.response);
		}
	});
	document.querySelector('h2#log').innerHTML = 'Build Log';
}

function expand_all(btn) {
	var logs = document.querySelectorAll('pre[data-src]');
	for (var i = 0; i < logs.length; i++) {
		var skips = logs[i].querySelectorAll('span.skip');
		if (skips.length > 0) {
			expand_skipped(skips);
		}
	}
	btn.remove();
}

function expand_element(els) {
	var parent = els[0].parentElement;
	var hidden = parent.querySelectorAll(".hidden");
	for (var i = 0; i < hidden.length; i++) {
		hidden[i].classList.toggle("hidden");
	}
	els[0].classList.add("hidden");
}

/* given a string containing ansi formatting directives, return a new one
   with designated regions of text marked with the appropriate color directives,
   and with all unknown directives stripped */
function ansi_to_html(orig) {
	// Given a cmd (like "32" or "0;97"), some enclosed body text, and the original string,
	// either return the body wrapped in an element to achieve the desired result, or the
	// original string if nothing works.
	function annotate(cmd, body, orig) {
		var code = +(cmd.replace('0;', ''));
		if (code === 0) // reset
			return body;
		else if (code === 1) // bold
			return '<em>' + body + '</em>';
		else if (30 <= code && code <= 37) // foreground color
			return '<span class="ansi-' + (code - 30) + '">' + body + '</span>'
		else if (90 <= code && code <= 97) // foreground color, bright
			return '<span class="ansi-' + (code - 90 + 8) + '">' + body + '</span>'
		return body;  // fallback: don't change anything
	}
	// Find commands, optionally followed by a bold command, with some content, then a reset command.
	// Unpaired commands are *not* handled here, but they're very uncommon.
	var filtered = orig.replace(/\033\[([0-9;]*)\w(\033\[1m)?([^\033]*?)\033\[0m/g, function(match, code, bold, body, offset, string) {
		if (bold !== undefined)  // normal code + bold
			return '<em>' + annotate(code, body, string) + '</em>';
		return annotate(code, body, string);
	})
	// Strip out anything left over.
	return filtered.replace(/\033\[([0-9;]*\w)/g, function(match, cmd, offset, string) {
		console.log('unhandled ansi code: ', cmd, "context:", JSON.stringify(filtered.slice(offset-50,offset+50)));
		return '';
	});
}

function fix_escape_codes() {
	var logs = document.querySelectorAll('pre[data-src]');
	for (var i = 0; i < logs.length; i++) {
		var orig = logs[i].innerHTML;
		var newer = ansi_to_html(orig);
		if (orig !== newer) {
			logs[i].innerHTML = newer;
		}
	}
}

/* Remove unicode sequences caused by colorized output in junit.xml */
function remove_unicode_escape_codes() {
	var errors = document.querySelectorAll('pre.error')
	for (var i = 0; i < errors.length; i++) {
		var orig = errors[i].innerHTML
		var newer = orig.replace(/\ufffd\[\d+m/g, "")
		if (orig !== newer) {
			errors[i].innerHTML = newer;
		}
	}
}

function init() {
	fix_timestamps();
	fix_escape_codes();
	remove_unicode_escape_codes();
	document.body.onclick = function(evt) {
		var target = evt.target;
		if (target.nodeName === 'SPAN' && target.classList.contains('skip')) {
			expand_skipped([target]);
			evt.preventDefault();
		}
		if (target.nodeName === 'SPAN' && target.classList.contains('expand')) {
			expand_element([target]);
			evt.preventDefault();
		}
	}
}

if (typeof module !== 'undefined' && module.exports) {
	// enable node.js `require('./build')` to work for testing
	module.exports = {
		ansi_to_html: ansi_to_html
	}
}

// Acknowledge a PR to suppress it. If repo is "CLEAR", clear acks instead.
function ack(event, repo, number, latest) {
	event.stopPropagation();
	var req = new XMLHttpRequest();
	req.open('POST', '/pr');
	req.onload = function(resp) {
		if (req.status != 200) return;
		var row = document.getElementById('needs-attention ' + repo + ' ' + number);
		if (row) {
			row.remove();
		}
	}
	req.send(JSON.stringify({
		command: 'ack',
		repo: repo,
		number: number,
		latest: latest,
	}));
}

// Reset the acknowledged PRs
function ack_clear() {
	var req = new XMLHttpRequest();
	req.open('POST', '/pr');
	req.onload = function(resp) {
		if (req.status != 200) return;
		document.location = document.location;  // refresh
	}
	req.send(JSON.stringify({
		command: 'ack-clear',
	}));
}
