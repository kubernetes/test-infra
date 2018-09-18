"use strict";

window.onload = function() {
    var infoDiv = document.getElementById("info-div");
    var infoH2 = infoDiv.getElementsByTagName("h4")[0];
    infoH2.addEventListener("click", infoToggle(infoDiv.getElementsByTagName("span")[0]), true);

    redraw();
};

function infoToggle(toToggle) {
    return function(event) {
        if (toToggle.className == "hidden") {
            toToggle.className = "";
            event.target.textContent = "Merge Requirements: (click to collapse)";
        } else {
            toToggle.className = "hidden";
            event.target.textContent = "Merge Requirements: (click to expand)";
        }
    }
}

function redraw() {
    redrawQueries();
    redrawPools();
}

function createLink(href, text) {
    var a = document.createElement("a");
    a.href = href;
    a.appendChild(document.createTextNode(text));
    return a;
}

/**
 * escapeLabel escaped label name that returns a valid name used for css
 * selector.
 * @param {string} label
 * @returns {string}
 */
function escapeLabel(label) {
  if (label === "") return "";
  const toUnicode = function(index) {
    const h = label.charCodeAt(index).toString(16).split('');
    while (h.length < 6) h.splice(0, 0, '0');

    return 'x' + h.join('');
  };
  let result = "";
  const alphaNum = /^[0-9a-zA-Z]+$/;

  for (let i = 0; i < label.length; i++) {
    const c = label.charCodeAt(i);
    if ((i === 0 && c > 47 && c < 58) || !label[i].match(alphaNum)) {
      result += toUnicode(i);
      continue;
    }
    result += label[i];
  }

  return result
}

/**
 * Creates a HTML element for the label given its name
 * @param label
 * @returns {HTMLElement}
 */
function createLabelEl(label) {
  const el = document.createElement("SPAN");
  const escapedName = escapeLabel(label);
  el.classList.add("mdl-shadow--2dp", "label", escapedName);
  el.textContent = label;

  return el;
}

function createStrong(text) {
    const s = document.createElement("STRONG");
    s.appendChild(document.createTextNode(text));
    return s;
}

function fillDetail(data, type, connector, container, styleData) {
    if (!data || (Array.isArray(data) && data.length === 0)) {
        return;
    }
    container.appendChild(createStrong(connector));
    container.appendChild(document.createTextNode(`the following ${type}: `));
    container.appendChild(document.createElement("br"));

    const ul = document.createElement("ul");
    const li = document.createElement("li");
    ul.appendChild(li);
    container.appendChild(ul);

    if (typeof data === 'string') {
        li.appendChild(document.createTextNode(data));
    } else  if (Array.isArray(data)) {
        for (let i = 0; i < data.length; i++) {
            const v = data[i];
            li.appendChild(styleData(v));
            if (i + 1 < data.length) {
                li.appendChild(document.createTextNode(" "));
            }
        }
    }
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
        var tideQuery = tideData.TideQueries[i];

        // create list entry for the query, all details will be within this element
        var li = document.createElement("li");

        // GitHub query search link
        var a = createLink(
            "https://github.com/search?utf8=" + encodeURIComponent("\u2713") + "&q=" + encodeURIComponent(query),
            "GitHub Search Link"
        );
        li.appendChild(a);

        // build the description
        // all queries should implicitly mean this
        const ul = document.createElement("ul");
        const innerLi = document.createElement("li");
        // add the list of repos, defaulting to an empty array if no repos have been provided.
        const repos = tideQuery["repos"] || [];
        if (repos.length > 0) {
            const explanationPrefix = " - Meaning: Is an open Pull Request " +
                "in one of the following repos: ";
            li.appendChild(document.createTextNode(explanationPrefix));
            for (let j = 0; j < repos.length; j++) {
                innerLi.appendChild(createLink("https://github.com/" + repos[j], repos[j]));
                if (j + 1 < repos.length) {
                    innerLi.appendChild(document.createTextNode(", "));
                }
            }
        } else if (tideQuery.orgs && tideQuery.orgs.length > 0) {
            const explanationPrefix = " - Meaning: Is an open Pull Request " +
                "in one of the following orgs: ";
            li.appendChild(document.createTextNode(explanationPrefix));
            for (let i = 0; i < tideQuery.orgs.length; i++) {
                const org = tideQuery.orgs[i];
                innerLi.appendChild(createLink("https://github.com/" + org, org));
                if (i + 1 < repos.length) {
                    innerLi.appendChild(document.createTextNode(", "));
                }
            }
        }
        ul.appendChild(innerLi);
        li.appendChild(ul);
        // required labels
        fillDetail(tideQuery.labels, "labels", "with ", li, function(data) {
          return createLabelEl(data);
        });
        // required to be not present labels
        fillDetail(tideQuery.missingLabels, "labels", "without ", li, function(data) {
            return createLabelEl(data);
        });
        // list milestone if existed
        fillDetail(tideQuery.milestone, "milestone", "with ", li, function(data) {
            return document.createTextNode(data);
        });
        // list all excluded branches
        fillDetail(tideQuery.excludedBranches, "branches", "exclude ", li, function(data) {
            return document.createTextNode(data);
        });
        // list all included branches
        fillDetail(tideQuery.includedBranches, "branches", "targeting ", li, function(data) {
            return document.createTextNode(data);
        });
        // GitHub native review required
        var reviewApprovedRequired = tideQuery.hasOwnProperty("reviewApprovedRequired") && tideQuery["reviewApprovedRequired"];
        if (reviewApprovedRequired) {
            li.appendChild(document.createTextNode("and must be "));
            li.appendChild(createLink(
                "https://help.github.com/articles/about-pull-request-reviews/",
                "approved by GitHub review"
            ));
        }

        // actually add the entry
        queries.appendChild(li);
    }
}

function redrawPools() {
    var pools = document.getElementById("pools").getElementsByTagName("tbody")[0];
    while (pools.firstChild)
        pools.removeChild(pools.firstChild);

    if (!tideData.Pools) {
        return;
    }
    for (var i = 0; i < tideData.Pools.length; i++) {
        var pool = tideData.Pools[i];
        var r = document.createElement("tr");


        var deckLink = "/?repo="+pool.Org+"%2F"+pool.Repo;
        var branchLink = "https://github.com/" + pool.Org + "/" + pool.Repo + "/tree/" + pool.Branch;
        var linksTD = document.createElement("td");
        linksTD.appendChild(createLink(deckLink, pool.Org + "/" + pool.Repo));
        linksTD.appendChild(document.createTextNode(" "));
        linksTD.appendChild(createLink(branchLink, pool.Branch));
        r.appendChild(linksTD);
        r.appendChild(createActionCell(pool));
        r.appendChild(createBatchCell(pool));
        r.appendChild(createPRCell(pool, pool.SuccessPRs));
        r.appendChild(createPRCell(pool, pool.PendingPRs));
        r.appendChild(createPRCell(pool, pool.MissingPRs));

        pools.appendChild(r);
    }
}

function createActionCell(pool) {
    var targeted = pool.Target && pool.Target.length;
    var blocked = pool.Blockers && pool.Blockers.length;
    var action = pool.Action.replace("_", " ");
    if (targeted || blocked) {
        action += ": "
    }
    var c = document.createElement("td");
    c.appendChild(document.createTextNode(action));

    if (blocked) {
        c.classList.add("blocked");
        addBlockersToElem(c, pool)
    } else if (targeted) {
        addPRsToElem(c, pool, pool.Target)
    }
    return c;
}

function createPRCell(pool, prs) {
    var c = document.createElement("td");
    addPRsToElem(c, pool, prs);
    return c;
}

function createBatchCell(pool) {
    var td = document.createElement('td');
    if (pool.BatchPending) {
        var numbers = pool.BatchPending.map(p => String(p.Number));
        var batchRef = pool.Branch + ',' + numbers.join(',');
        var href = '/?repo=' + encodeURIComponent(pool.Org + '/' + pool.Repo) +
            '&type=batch&pull=' + encodeURIComponent(batchRef);
        var link = document.createElement('a');
        link.href = href;
        link.appendChild(document.createTextNode(numbers.join(' ')));
        td.appendChild(link);
    }
    return td;
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

// addBlockersToElem adds a space separated list of Issue numbers that link to the
// corresponding Issues on github that are blocking merge.
function addBlockersToElem(elem, pool) {
    if (!pool.Blockers) {
        return;
    }
    for (var i = 0; i < pool.Blockers.length; i++) {
        var b = pool.Blockers[i];
        var id = "blocker-" + pool.Org + "-" + pool.Repo + "-" + b.Number;
        var a = document.createElement("a");
        a.href = b.URL;
        a.appendChild(document.createTextNode("#" + b.Number));
        a.id = id;
        addToolTipToElem(a, document.createTextNode(b.Title));


        elem.appendChild(a);
        // Add a space after each PR number except the last.
        if (i+1 < pool.Blockers.length) {
            elem.appendChild(document.createTextNode(" "));
        }
    }
}

function addToolTipToElem(elem, tipElem) {
    var tooltip = document.createElement("div");
    tooltip.appendChild(tipElem);
    tooltip.setAttribute("data-mdl-for", elem.id);
    tooltip.classList.add("mdl-tooltip", "mdl-tooltip--large");
    elem.appendChild(tooltip);
}
