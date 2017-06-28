"use strict";

var types = {};
var repos = {};
var jobs = {};
var authors = {};
var pulls = {};
var states = {};

function getParameterByName(name) {  // http://stackoverflow.com/a/5158301/3694
    var match = RegExp('[?&]' + name + '=([^&/]*)').exec(window.location.search);
    return match && decodeURIComponent(match[1].replace(/\+/g, ' '));
}

function updateQueryStringParameter(uri, key, value) {
    var re = new RegExp("([?&])" + key + "=.*?(&|$)", "i");
    var separator = uri.indexOf('?') !== -1 ? "&" : "?";
    if (uri.match(re)) {
            return uri.replace(re, '$1' + key + "=" + value + '$2');
    } else {
        return uri + separator + key + "=" + value;
    }
}

window.onload = function() {
    for (var i = 0; i < allBuilds.length; i++) {
        types[allBuilds[i].type] = true;
        repos[allBuilds[i].repo] = true;
        jobs[allBuilds[i].job] = true;
        if (allBuilds[i].type === "presubmit") {
            authors[allBuilds[i].author] = true;
            pulls[allBuilds[i].number] = true;
            states[allBuilds[i].state] = true;
        }
    }

    var ts = Array.from(Object.keys(types)).sort();
    addOptions(ts, "type");
    var rs = Array.from(Object.keys(repos)).filter(function(r) { return r !== "/"; }).sort();
    addOptions(rs, "repo");
    var js = Array.from(Object.keys(jobs)).sort();
    addOptions(js, "job");
    var as = Array.from(Object.keys(authors)).sort(function (a, b) {
        return a.toLowerCase().localeCompare(b.toLowerCase());
    });
    addOptions(as, "author");
    var ps = Array.from(Object.keys(pulls)).sort(function (a, b) {
        return parseInt(a) - parseInt(b);
    });
    addOptions(ps, "pull");
    var ss = Array.from(Object.keys(states)).sort();
    addOptions(ss, "state");

    redraw();
};

function addOptions(s, p) {
    var sel = document.getElementById(p);
    var param = getParameterByName(p);
    for (var i = 0; i < s.length; i++) {
        var o = document.createElement("option");
        o.text = s[i];
        if (param && s[i] === param) {
            o.selected = true;
        }
        sel.appendChild(o);
    }
}

function equalSelected(sel, t) {
    return sel.selectedIndex == 0 || sel.options[sel.selectedIndex].text == t;
}

function encodedText(sel) {
    if (sel.selectedIndex == 0) {
        return "";
    }
    return encodeURIComponent(sel.options[sel.selectedIndex].text);
}

function groupKey(build) {
    return build.repo + " " + build.number + " " + build.refs;
}

function redraw() {
    var modal = document.getElementById('rerun');
    var rerun_command = document.getElementById('rerun-content');
    window.onclick = function(event) {
        if (event.target == modal) {
            modal.style.display = "none";
        }
    };
    var builds = document.getElementById("builds").getElementsByTagName("tbody")[0];
    while (builds.firstChild)
        builds.removeChild(builds.firstChild);

    var typeSel = document.getElementById("type")
    var repoSel = document.getElementById("repo")
    var pullSel = document.getElementById("pull")
    var authorSel = document.getElementById("author")
    var jobSel = document.getElementById("job")
    var stateSel = document.getElementById("state")

    if (window.history && window.history.replaceState !== undefined) {
        var args = [];
        var tt = encodedText(typeSel);
        if (tt !== "") args.push("type=" + tt);
        var rt = encodedText(repoSel);
        if (rt !== "") args.push("repo=" + rt);
        var pt = encodedText(pullSel);
        if (pt !== "") args.push("pull=" + pt);
        var at = encodedText(authorSel);
        if (at !== "") args.push("author=" + at);
        var jt = encodedText(jobSel);
        if (jt !== "") args.push("job=" + jt);
        var st = encodedText(stateSel);
        if (st !== "") args.push("state=" + st);
        if (args.length > 0) {
            history.replaceState(null, "", "/?" + args.join('&'));
        } else {
            history.replaceState(null, "", "/")
        }
    }

    var lastKey = '';
    for (var i = 0, emitted = 0; i < allBuilds.length && emitted < 500; i++) {
        var build = allBuilds[i];
        if (!equalSelected(typeSel, build.type)) continue;

        if (build.type !== "periodic" && !equalSelected(repoSel, build.repo)) continue;
        if (!equalSelected(stateSel, build.state)) continue;
        if (!equalSelected(jobSel, build.job)) continue;
        if (build.type === "presubmit") {
            if (!equalSelected(pullSel, build.number)) continue;
            if (!equalSelected(authorSel, build.author)) continue;
        }
        emitted++;

        var r = document.createElement("tr");
        r.appendChild(stateCell(build.state));
        if (build.pod_name) {
            r.appendChild(createLinkCell("\u2261", "log?pod=" + build.pod_name, "Build log."));
        } else {
            r.appendChild(createTextCell(""));
        }
        r.appendChild(createRerunCell(modal, rerun_command, build.prow_job));
        var key = groupKey(build);
        if (key !== lastKey) {
            // This is a different PR or commit than the previous row.
            lastKey = key;
            r.className = "changed";

            if (build.type === "periodic") {
                r.appendChild(createTextCell(""));
            } else {
                r.appendChild(createLinkCell(build.repo, "https://github.com/" + build.repo, ""));
            }
            if (build.type === "presubmit") {
                r.appendChild(prRevisionCell(build));
            } else if (build.type === "batch") {
                r.appendChild(batchRevisionCell(build));
            } else if (build.type === "postsubmit") {
                r.appendChild(pushRevisionCell(build));
            } else if (build.type === "periodic") {
                r.appendChild(createTextCell(""));
            }
        } else {
            // Don't render identical cells for the same PR/commit.
            r.appendChild(createTextCell(""));
            r.appendChild(createTextCell(""));
        }
        if (build.url === "") {
            r.appendChild(createTextCell(build.job));
        } else {
            r.appendChild(createLinkCell(build.job, build.url, ""));
        }
        r.appendChild(createTextCell(build.started));
        r.appendChild(createTextCell(build.duration));
        builds.appendChild(r);
    }
}

function createTextCell(text) {
    var c = document.createElement("td");
    c.appendChild(document.createTextNode(text));
    return c;
}

function createLinkCell(text, url, title) {
    var c = document.createElement("td");
    var a = document.createElement("a");
    a.href = url;
    if (title !== "") {
        a.title = title;
    }
    a.appendChild(document.createTextNode(text));
    c.appendChild(a);
    return c;
}

function createRerunCell(modal, rerun_command, prowjob) {
    var url = "https://" + window.location.hostname + "/rerun?prowjob=" + prowjob;
    var c = document.createElement("td");
    var a = document.createElement("a");
    a.href = "#";
    a.title = "Show instructions for rerunning this job.";
    a.onclick = function() {
        modal.style.display = "block";
        rerun_command.textContent = "kubectl create -f \"" + url + "\"";
    };
    a.appendChild(document.createTextNode("\u27F3"));
    c.appendChild(a);
    return c;
}

function stateCell(state) {
    var c = document.createElement("td");
    c.className = state;
    if (state === "triggered" || state === "pending") {
        c.appendChild(document.createTextNode("\u2022"));
    } else if (state === "success") {
        c.appendChild(document.createTextNode("\u2713"));
    } else if (state === "failure" || state === "error" || state === "aborted") {
        c.appendChild(document.createTextNode("\u2717"));
    }
    return c;
}

function batchRevisionCell(build) {
    var c = document.createElement("td");
    var pr_refs = build.refs.split(",");
    for (var i = 1; i < pr_refs.length; i++) {
        if (i != 1) c.appendChild(document.createTextNode(", "));
        var pr = pr_refs[i].split(":")[0];
        var l = document.createElement("a");
        l.href = "https://github.com/" + build.repo + "/pull/" + pr;
        l.text = pr;
        c.appendChild(document.createTextNode("#"));
        c.appendChild(l);
    }
    return c;
}

function pushRevisionCell(build) {
    var c = document.createElement("td");
    var bl = document.createElement("a");
    bl.href = "https://github.com/" + build.repo + "/tree/" + build.base_sha;
    bl.text = build.base_ref + " (" + build.base_sha.slice(0, 7) + ")";
    c.appendChild(bl);
    return c;
}

function prRevisionCell(build) {
    var c = document.createElement("td");
    c.appendChild(document.createTextNode("#"));
    var pl = document.createElement("a");
    pl.href = "https://github.com/" + build.repo + "/pull/" + build.number;
    pl.text = build.number;
    c.appendChild(pl);
    c.appendChild(document.createTextNode(" ("));
    var cl = document.createElement("a");
    cl.href = "https://github.com/" + build.repo + "/pull/" + build.number + '/commits/' + build.pull_sha;
    cl.text = build.pull_sha.slice(0, 7);
    c.appendChild(cl);
    c.appendChild(document.createTextNode(") by "));
    var al = document.createElement("a");
    al.href = "https://github.com/" + build.author;
    al.text = build.author;
    c.appendChild(al);
    return c;
}
