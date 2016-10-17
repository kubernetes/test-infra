var repos = {};
var jobs = {};

window.onload = function() {
    for (var i = 0; i < allBuilds.length; i++) {
        repos[allBuilds[i].repo] = true;
        jobs[allBuilds[i].job] = true;
    }

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

    for (var i = 0; i < allBuilds.length; i++) {
        if (!repos[allBuilds[i].repo])
            continue;
        if (!jobs[allBuilds[i].job])
            continue;
        if (!String(allBuilds[i].author).includes(author)) {
            continue;
        }
        if (!String(allBuilds[i].number).includes(pr)) {
            continue;
        }

        var r = document.createElement("tr");
        r.appendChild(stateCell(allBuilds[i].state));
        r.appendChild(createLinkCell(allBuilds[i].repo, "https://github.com/" + allBuilds[i].repo));
        r.appendChild(createLinkCell(allBuilds[i].number, "https://github.com/" + allBuilds[i].repo + "/pull/" + allBuilds[i].number));
        r.appendChild(createLinkCell(allBuilds[i].author, "https://github.com/" + allBuilds[i].author));
        r.appendChild(createTextCell(allBuilds[i].job));
        if (allBuilds[i].url === "") {
            r.appendChild(createTextCell(allBuilds[i].description));
        } else {
            r.appendChild(createLinkCell(allBuilds[i].description, allBuilds[i].url));
        }
        r.appendChild(createTextCell(allBuilds[i].started));
        r.appendChild(createTextCell(allBuilds[i].finished));
        r.appendChild(createTextCell(allBuilds[i].duration));
        builds.appendChild(r);
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
