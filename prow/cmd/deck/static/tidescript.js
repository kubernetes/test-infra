"use strict";

window.onload = function() {
    redraw();
};

function redraw() {
    redrawQueries();
    redrawPools();
}

function redrawQueries() {
    var queries = document.getElementById("queries");
    while (queries.firstChild)
        queries.removeChild(queries.firstChild);

    if (!tideData.Queries) {
        return;
    }
    for (var i = 0; i < tideData.Queries.length; i++) {
        var query = tideData.Queries[i];

        var a = document.createElement("a");
        a.href = "https://github.com/search?utf8=" + encodeURIComponent("\u2713") + "&q=" + encodeURIComponent(query);
        a.appendChild(document.createTextNode(query));

        //var div = document.createElement("div");
        //div.appendChild(a);
        var li = document.createElement("li");
        li.appendChild(a);

        queries.appendChild(li);
    }
}

function redrawPools() {
    var pools = document.getElementById("pools").getElementsByTagName("tbody")[0];
    while (pools.firstChild)
        pools.removeChild(pools.firstChild);

    // TODO(spxtr): Sort these.
    if (!tideData.Pools) {
        return;
    }
    for (var i = 0; i < tideData.Pools.length; i++) {
        var pool = tideData.Pools[i];
        var r = document.createElement("tr");

        var repoName = pool.Org + "/" + pool.Repo + " " + pool.Branch;
        var repoLink = "https://github.com/" + pool.Org + "/" + pool.Repo + "/tree/" + pool.Branch;
        r.appendChild(createLinkCell(repoName, repoLink, ""));
        r.appendChild(createActionCell(pool));
        r.appendChild(createPRCell(pool, pool.BatchPending));
        r.appendChild(createPRCell(pool, pool.SuccessPRs));
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

function createActionCell(pool) {
    var action = pool.Action;
    var targetted = pool.Target && pool.Target.length
    var c = document.createElement("td");

    if (targetted) {
        action += ": "
    }
    c.appendChild(document.createTextNode(action));
    if (targetted) {
        addPRsToElem(c, pool, pool.Target)
    }
    return c;
}

function createPRCell(pool, prs) {
    var c = document.createElement("td");
    addPRsToElem(c, pool, prs)
    return c;
}

// addPRsToElem adds a space separated list of PR numbers that link to the corresponding PR on github.
function addPRsToElem(elem, pool, prs) {
    if (prs) {
        for (var i = 0; i < prs.length; i++) {
            var a = document.createElement("a");
            a.href = "https://github.com/" + pool.Org + "/" + pool.Repo + "/pull/" + prs[i].Number;
            a.appendChild(document.createTextNode("#" + prs[i].Number));
            elem.appendChild(a);
            // Add a space after each PR number except the last.
            if (i+1 < prs.length) {
                elem.appendChild(document.createTextNode(" "));
            }
        }
    }
}