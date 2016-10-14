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

function redraw() {
    var builds = document.getElementById("builds").getElementsByTagName("tbody")[0];
    while (builds.firstChild)
        builds.removeChild(builds.firstChild);

    for (var i = 0; i < allBuilds.length; i++) {
        if (!repos[allBuilds[i].repo])
            continue;
        if (!jobs[allBuilds[i].job])
            continue;

        var r = document.createElement("tr");
        r.appendChild(stateCell(allBuilds[i].state));
        r.appendChild(createCell(allBuilds[i].repo));
        r.appendChild(createCell(allBuilds[i].number));
        r.appendChild(createCell(allBuilds[i].job));
        r.appendChild(createCell(allBuilds[i].description));
        r.appendChild(createCell(allBuilds[i].started));
        r.appendChild(createCell(allBuilds[i].finished));
        r.appendChild(createCell(allBuilds[i].duration));
        builds.appendChild(r);
    }
}

function createCell(text) {
    var c = document.createElement("td");
    c.appendChild(document.createTextNode(text));
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
