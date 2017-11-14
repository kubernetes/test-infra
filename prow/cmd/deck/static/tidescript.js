"use strict";

window.onload = function() {
    redraw();
};

function redraw() {
    var pools = document.getElementById("pools").getElementsByTagName("tbody")[0];
    while (pools.firstChild)
        pools.removeChild(pools.firstChild);

    // TODO(spxtr): Sort these.
    for (var i = 0; i < allPools.length; i++) {
        var pool = allPools[i];
        var r = document.createElement("tr");

        var repoName = pool.Org + "/" + pool.Repo + " " + pool.Branch;
        var repoLink = "https://github.com/" + pool.Org + "/" + pool.Repo + "/tree/" + pool.Branch;
        r.appendChild(createLinkCell(repoName, repoLink, ""));
        r.appendChild(createTextCell(pool.Action));
        r.appendChild(createPRCell(pool, pool.BatchPending));
        r.appendChild(createPRCell(pool, pool.PassingPRs));
        r.appendChild(createPRCell(pool, pool.PendingPRs));
        r.appendChild(createPRCell(pool, pool.MissingPRs));

        pools.appendChild(r);
    }
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

function createTextCell(text) {
    var c = document.createElement("td");
    c.appendChild(document.createTextNode(text));
    return c;
}

function createPRCell(pool, prs) {
    var c = document.createElement("td");
    if (prs) {
        for (var i = 0; i < prs.length; i++) {
            var a = document.createElement("a");
            a.href = "https://github.com/" + pool.Org + "/" + pool.Repo + "/pull/" + prs[i].Number;
            a.appendChild(document.createTextNode("#" + prs[i].Number));
            c.appendChild(a);
            c.appendChild(document.createTextNode(" "));
        }
    }
    return c;
}
