var repos = {};
var jobs = {};

var maxLength = 500;

function getParameterByName(name) {  // http://stackoverflow.com/a/5158301/3694
    var match = RegExp('[?&]' + name + '=([^&/]*)').exec(window.location.search);
    return match && decodeURIComponent(match[1].replace(/\+/g, ' '));
}

window.onload = function() {
    for (var i = 0; i < allBuilds.length; i++) {
        repos[allBuilds[i].repo] = true;
        jobs[allBuilds[i].job] = true;
    }

    setValueFromParameter = function(name) {
        var value = getParameterByName(name);
        if (value) {
            document.getElementById(name)[name === "batch" ? 'checked' : 'value'] = value;
        }
    }

    setValueFromParameter("pr");
    setValueFromParameter("author");
    setValueFromParameter("refs");
    setValueFromParameter("batch");

    var rs = Array.from(Object.keys(repos)).sort();
    for (var i = 0; i < rs.length; i++) {
        var l = document.createElement("label");
        var c = document.createElement("input");
        c.setAttribute("type", "checkbox");
        c.setAttribute("checked", true);
        c.onclick = (function(repo) {
            return function() {
                repos[repo] = this.checked;
                redrawAllRepos();
                redraw();
            }
        })(rs[i]);
        l.appendChild(c);
        l.appendChild(document.createTextNode(rs[i]));
        document.getElementById("repos").appendChild(l);
        document.getElementById("repos").appendChild(document.createElement("br"));
    }

    var js = Array.from(Object.keys(jobs)).sort();
    for (var i = 0; i < js.length; i++) {
        var l = document.createElement("label");
        var c = document.createElement("input");
        c.setAttribute("type", "checkbox");
        c.setAttribute("checked", true);
        c.onclick = (function(job) {
            return function() {
                jobs[job] = this.checked;
                redrawAllJobs();
                redraw();
            }
        })(js[i]);
        l.appendChild(c);
        l.appendChild(document.createTextNode(js[i]));
        document.getElementById("jobs").appendChild(l);
        document.getElementById("jobs").appendChild(document.createElement("br"));
    }

    redraw();
};

function redrawAllRepos() {
    var allRepos = document.getElementById("allRepos");
    var boxes = document.getElementById("repos").getElementsByTagName("input");
    var checked = 0;
    var unchecked = 0;
    for (var i = 0; i < boxes.length; i++) {
        if (boxes[i].id == "allRepos") {
            continue;
        } else if (boxes[i].checked) {
            checked++;
        } else {
            unchecked++;
        }
    }
    if (checked == 0) {
        allRepos.indeterminate = false;
        allRepos.checked = false;
    } else if (unchecked == 0) {
        allRepos.indeterminate = false;
        allRepos.checked = true;
    } else {
        allRepos.indeterminate = true;
    }
}

function toggleRepos(el) {
    var boxes = document.getElementById("repos").getElementsByTagName("input");
    for (var i = 0; i < boxes.length; i++) {
        boxes[i].checked = el.checked;
    }
    for (var repo in repos) {
        repos[repo] = el.checked;
    }
    redraw();
}

function redrawAllJobs() {
    var allRepos = document.getElementById("allJobs");
    var boxes = document.getElementById("jobs").getElementsByTagName("input");
    var checked = 0;
    var unchecked = 0;
    for (var i = 0; i < boxes.length; i++) {
        if (boxes[i].id == "allJobs") {
            continue;
        } else if (boxes[i].checked) {
            checked++;
        } else {
            unchecked++;
        }
    }
    if (checked == 0) {
        allJobs.indeterminate = false;
        allJobs.checked = false;
    } else if (unchecked == 0) {
        allJobs.indeterminate = false;
        allJobs.checked = true;
    } else {
        allJobs.indeterminate = true;
    }
}


function toggleJobs(el) {
    var boxes = document.getElementById("jobs").getElementsByTagName("input");
    for (var i = 0; i < boxes.length; i++) {
        boxes[i].checked = el.checked;
    }
    for (var job in jobs) {
        jobs[job] = el.checked;
    }
    redraw();
}

function redraw() {
    var builds = document.getElementById("builds").getElementsByTagName("tbody")[0];
    while (builds.firstChild)
        builds.removeChild(builds.firstChild);

    var author = document.getElementById("author").value;
    var pr = document.getElementById("pr").value;
    var refs = document.getElementById("refs").value;
    var batch = document.getElementById("batch").checked;

    if (history && history.replaceState !== undefined) {
        var args = [];
        if (author) args.push('author=' + author);
        if (pr)     args.push('pr=' + pr);
        if (refs)   args.push('refs=' + refs);
        if (batch)  args.push('batch=' + batch);
        if (args.length > 0) {
            history.replaceState(null, "", "/?" + args.join('&'));
        } else {
            history.replaceState(null, "", "/")
        }
    }

    var refFilter = function() { return true; };
    if (refs) {
        var regex = new RegExp(refs.replace(/,/g, '.*,'));
        refFilter = regex.test.bind(regex);
    }

    var firstBuild = undefined;
    var emitted = 0;

    for (var i = 0; i < allBuilds.length && emitted < 500; i++) {
        var build = allBuilds[i];
        if (!repos[build.repo])
            continue;
        if (!jobs[build.job])
            continue;
        if (batch && build.type !== "batch")
            continue;
        if (!String(build.number).includes(pr))
            continue;
        if (!String(build.author).includes(author))
            continue;
        if (!refFilter(build.refs))
            continue;

        emitted++;

        if (!firstBuild)
            firstBuild = build;

        var r = document.createElement("tr");
        r.appendChild(stateCell(build.state));
        if (build.pod_name !== "") {
            r.appendChild(createLinkCell("\u2261", "log?pod=" + build.pod_name));
        } else {
            r.appendChild(createTextCell(""));
        }
        r.appendChild(createLinkCell(build.repo, "https://github.com/" + build.repo));
        if (build.type == "batch") {
            var batchText = createTextCell("Batch");
            batchText.setAttribute("title", build.refs.replace(/,/g, ' '));
            r.appendChild(batchText);
            r.appendChild(createTextCell(''));
        } else {
            r.appendChild(createLinkCell(build.number, "https://github.com/" + build.repo + "/pull/" + build.number));
            r.appendChild(createLinkCell(build.author, "https://github.com/" + build.author));
        }
        r.appendChild(createTextCell(build.job));
        if (build.url === "") {
            r.appendChild(createTextCell(build.description));
        } else {
            r.appendChild(createLinkCell(build.description, build.url));
        }
        r.appendChild(createTextCell(build.started));
        r.appendChild(createTextCell(build.finished));
        r.appendChild(createTextCell(build.duration));
        builds.appendChild(r);
    }

    var batch_desc = document.getElementById("batch-desc");
    if (refs && firstBuild) {
        batch_desc.innerHTML = 'Batch PRs:';  // clear
        batch_desc.style = ''
        var prs = refs.replace(/(?:(\d+)|[^:]+):[^,]*,?/g, '$1 ').trim().split(' ');
        for (var i = 0; i < prs.length; i++) {
            var pr = prs[i];
            batch_desc.appendChild(createLinkCell(pr, "https://github.com/" + firstBuild.repo + "/pull/" + pr))
        }
    } else {
        batch_desc.style = "display: none";
    }
}

function createTextCell(text) {
    var c = document.createElement("td");
    c.appendChild(document.createTextNode(text));
    return c;
}

function createLinkCell(text, url) {
    var c = document.createElement("td");
    var a = document.createElement("a");
    a.href = url;
    a.appendChild(document.createTextNode(text));
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
