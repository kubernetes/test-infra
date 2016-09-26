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
	replace('shorttimestamp', 'DD HH:mm')
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

var gcs_cache = {};

// Download a file from GCS, and run "callback" with its contents.
function gcs_get(gcs_path, callback) {
	if (gcs_cache[gcs_path]) {
		callback(gcs_cache[gcs_path]);
		return;
	}
	// Matches gs://bucket/file/path -> [..., "bucket", "file/path"]
	//         /bucket/file/path -> [..., "bucket", "file/path"]
	var groups = gcs_path.match(/([^/:]+)\/(.*)/);
	var bucket = groups[1], path = groups[2];
	var req = new XMLHttpRequest();
	req.open('GET',
		'https://www.googleapis.com/storage/v1/b/' + bucket + '/o/' +
		encodeURIComponent(path) + '?alt=media');
	req.onload = function(resp) {
		gcs_cache[gcs_path] = req.response;
		callback(req.response);
	}
	req.send();
}

function expand_skipped(els) {
	var src = els[0].parentElement.dataset['src'];
	gcs_get(src, function(data) {
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
	});
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

function init() {
	fix_timestamps();
	document.body.onclick = function(evt) {
		var target = evt.target;
		if (target.nodeName == 'SPAN' && target.className == 'skip') {
			expand_skipped([target]);
			evt.preventDefault();
		}
	}
}