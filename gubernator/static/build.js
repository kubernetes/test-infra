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

function show_skipped(ID) {
	document.getElementById(ID).style.display = "block"; 
}

