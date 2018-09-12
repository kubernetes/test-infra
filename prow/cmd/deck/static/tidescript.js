"use strict";

window.onload = function() {
    var infoDiv = document.getElementById("info-div");
    var infoH2 = infoDiv.getElementsByTagName("h4")[0];
    infoH2.addEventListener("click", infoToggle(infoDiv.getElementsByTagName("span")[0]), true);

    redraw();
};

document.addEventListener("DOMContentLoaded", function(event) {
   configure();
});

function configure() {
    if(!branding){
        return;
    }
    if (branding.logo) {
        document.getElementById('img').src = branding.logo;
    }
    if (branding.favicon) {
        document.getElementById('favicon').href = branding.favicon;
    }
    if (branding.background_color) {
        document.body.style.background = branding.background_color;
    }
    if (branding.header_color) {
        document.getElementsByTagName('header')[0].style.backgroundColor = branding.header_color;
    }
}

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

function createSpan(classList, style, text) {
    var s = document.createElement("span");
    s.classList.add(...classList);
    s.style = style;
    s.appendChild(document.createTextNode(text));
    return s;
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
        var explanationPrefix = " - Meaning: Is an open Pull Request in one of the following repos: ";
        li.appendChild(document.createTextNode(explanationPrefix));
        var ul = document.createElement("ul");
        var innerLi = document.createElement("li");
        ul.appendChild(innerLi);
        li.appendChild(ul);
        // add the list of repos, defaulting to an empty array if no repos have been provided.
        var repos = tideQuery["repos"] || [];
        for (var j = 0; j < repos.length; j++) {
            innerLi.appendChild(createLink("https://github.com/" + repos[j], repos[j]));
            if (j+1 < repos.length) {
                innerLi.appendChild(document.createTextNode(", "));
            }
        }
        // required labels
        var hasLabels = tideQuery.hasOwnProperty("labels") && tideQuery["labels"].length > 0;
        if (hasLabels) {
            var labels = tideQuery["labels"];
            li.appendChild(createSpan(["emphasis"], "", "with"));
            li.appendChild(document.createTextNode(" the following labels: "));
            li.appendChild(document.createElement("br"));
            var ul = document.createElement("ul");
            var innerLi = document.createElement("li");
            ul.appendChild(innerLi);
            li.appendChild(ul);
            for (var j = 0; j < labels.length; j++) {
                var label = labels[j];
                innerLi.appendChild(createLabelEl(label));
                if (j+1 < labels.length) {
                    innerLi.appendChild(document.createTextNode(" "));
                }
            }
        }
        // required to be not present labels
        var hasMissingLabels = tideQuery.hasOwnProperty("missingLabels") && tideQuery["missingLabels"].length > 0;
        if (hasMissingLabels) {
            var missingLabels = tideQuery["missingLabels"];
            if (hasLabels) {
                li.appendChild(createSpan(["emphasis"], "", "and without"));
            } else {
                li.appendChild(createSpan(["emphasis"], "", "without"));
            }
            li.appendChild(document.createTextNode(" the following labels: "));
            li.appendChild(document.createElement("br"));
            var ul = document.createElement("ul");
            var innerLi = document.createElement("li");
            ul.appendChild(innerLi);
            li.appendChild(ul);
            for (var j = 0; j < missingLabels.length; j++) {
                var label = missingLabels[j];
                innerLi.appendChild(createLabelEl(label));
                if (j+1 < missingLabels.length) {
                    innerLi.appendChild(document.createTextNode(" "));
                }
            }
        }

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
    addPRsToElem(c, pool, prs)
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
    var bs = pool.Blockers
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
