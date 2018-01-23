"use strict";

function getParameterByName(name) {  // http://stackoverflow.com/a/5158301/3694
    var match = RegExp('[?&]' + name + '=([^&/]*)').exec(
        window.location.search);
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

function optionsForRepo(repo) {
    var opts = {
        types: {},
        repos: {},
        jobs: {},
        authors: {},
        pulls: {},
        states: {},
    };

    for (var i = 0; i < allBuilds.length; i++) {
        var build = allBuilds[i];
        opts.types[build.type] = true;
        opts.repos[build.repo] = true;
        if (!repo || repo === build.repo) {
            opts.jobs[build.job] = true;
            if (build.type === "presubmit") {
                opts.authors[build.author] = true;
                opts.pulls[build.number] = true;
                opts.states[build.state] = true;
            }
        }
    }

    return opts;
}

function redrawOptions(opts) {
    var ts = Object.keys(opts.types).sort();
    addOptions(ts, "type");
    var rs = Object.keys(opts.repos).filter(function (r) {
        return r !== "/";
    }).sort();
    addOptions(rs, "repo");
    var js = Object.keys(opts.jobs).sort();
    addOptionFuzzySearch(js, "job", "job-input", "job-list");
    var as = Object.keys(opts.authors).sort(function (a, b) {
        return a.toLowerCase().localeCompare(b.toLowerCase());
    });
    addOptions(as, "author");
    var ps = Object.keys(opts.pulls).sort(function (a, b) {
        return parseInt(a) - parseInt(b);
    });
    addOptions(ps, "pull");
    var ss = Object.keys(opts.states).sort();
    addOptions(ss, "state");
};

function adjustScroll(el) {
    var parent = el.parentElement;
    var parentRect = parent.getBoundingClientRect();
    var elRect = el.getBoundingClientRect();

    if (elRect.top < parentRect.top) {
        parent.scrollTop -= elRect.height;
    } else if (elRect.top + elRect.height >= parentRect.top
        + parentRect.height) {
        parent.scrollTop += elRect.height;
    }
}

function handleDownKey() {
    var activeSearches =
        document.getElementsByClassName("active-fuzzy-search");
    if (activeSearches !== null && activeSearches.length !== 1) {
        return;
    }
    var activeSearch = activeSearches[0];
    if (activeSearch.tagName !== "UL" ||
        activeSearch.childElementCount === 0) {
        return;
    }

    var selectedJobs = activeSearch.getElementsByClassName("job-selected");
    if (selectedJobs.length > 1) {
        return;
    }
    if (selectedJobs.length === 0) {
        // If no job selected, selecte the first one that visible in the list.
        var activeSearchRect = activeSearch.getBoundingClientRect();
        var jobs = Array.from(activeSearch.children)
            .filter(function (elChild) {
                var childRect = elChild.getBoundingClientRect();
                var listRect = activeSearch.getBoundingClientRect();
                return childRect.top >= listRect.top &&
                    (childRect.top < listRect.top + listRect.height);
            });
        if (jobs.length === 0) {
            return;
        }
        jobs[0].classList.add("job-selected");
        return;
    }
    var selectedJob = selectedJobs[0];
    var nextSibling = selectedJob.nextElementSibling;
    if (!nextSibling) {
        return;
    }

    selectedJob.classList.remove("job-selected");
    nextSibling.classList.add("job-selected");
    adjustScroll(nextSibling);
}

function handleUpKey() {
    var activeSearches =
        document.getElementsByClassName("active-fuzzy-search");
    if (activeSearches && activeSearches.length !== 1) {
        return;
    }
    var activeSearch = activeSearches[0];
    if (activeSearch.tagName !== "UL" ||
        activeSearch.childElementCount === 0) {
        return;
    }

    var selectedJobs = activeSearch.getElementsByClassName("job-selected");
    if (selectedJobs.length !== 1) {
        return;
    }

    var selectedJob = selectedJobs[0];
    var previousSibling = selectedJob.previousElementSibling;
    if (!previousSibling) {
        return;
    }

    selectedJob.classList.remove("job-selected");
    previousSibling.classList.add("job-selected");
    adjustScroll(previousSibling);
}

window.onload = function () {
    document.addEventListener("keydown", function (event) {
        if (event.keyCode === 40) {
            handleDownKey();
        } else if (event.keyCode === 38) {
            handleUpKey();
        }
    });
    // set dropdown based on options from query string
    var opts = optionsForRepo("");
    initFuzzySearch(
        "job",
        "job-input",
        "job-list",
        Object.keys(opts["jobs"]).sort());
    redrawOptions(opts);
    redraw();
};

document.addEventListener("DOMContentLoaded", function (event) {
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
      document.getElementsByTagName(
          'header')[0].style.backgroundColor = branding.header_color;
    }
}

function displayFuzzySearchResult(el, inputContainer) {
    el.classList.add("active-fuzzy-search");
    el.style.top = inputContainer.height + "px";
    el.style.width = inputContainer.width + "px";
    el.style.height = 200 + "px";
}

function fuzzySearch(id, inputId, listId, data, inputValue) {
    var result = [];
    inputValue = inputValue.trim();

    if (inputValue === "") {
        addOptionFuzzySearch(data, id, inputId, listId, true);
        return;
    }

    for (var i = 0; i < data.length; i++) {
        if (data[i].includes(inputValue)) {
            result.push(data[i]);
        }
    }
    addOptionFuzzySearch(result, id, inputId, listId, true);
}

function validToken(token) {
    // 0-9
    if (token >= 48 && token <= 57) {
        return true;
    }
    // a-z
    if (token >= 65 && token <= 90) {
        return true;
    }
    // - and backspace
    return token === 189 || token === 8;
}

function handleEnterKeyDown(inputId, listId) {
    var input = document.getElementById(inputId);
    var list = document.getElementById(listId);
    if (!input || !list) {
        return;
    }

    if (list.childElementCount === 0) {
        return;
    }

    var selectedJobs = list.getElementsByClassName("job-selected");
    var job = list.firstElementChild.innerHTML;
    if (selectedJobs && selectedJobs.length === 1) {
        job = selectedJobs[0].innerHTML;
    }

    input.value = job;
    input.blur();
    list.classList.remove("active-fuzzy-search");
    redraw();
}

function registerFuzzySearchHandler(id, inputId, listId, data) {
    var input = document.getElementById(inputId);
    if (!input) {
        return;
    }

    input.addEventListener("keydown", function () {
        if (event.keyCode === 13) {
            // If enter key is hit, selects the first job in the list.
            handleEnterKeyDown(inputId, listId);
        } else if (validToken(event.keyCode)) {
            // Delay 1 frame that the input character is recorded before getting
            // input value
            setTimeout(function () {
                fuzzySearch(id, inputId, listId, data, input.value);
            }, 32);
        }
    });
}

function initFuzzySearch(id, inputId, listId, data) {
    var el = document.getElementById(id);
    var input = document.getElementById(inputId);
    var list = document.getElementById(listId);

    if (!input || !list || !el) {
        return;
    }

    list.classList.remove("active-fuzzy-search");
    input.addEventListener("focus", function () {
        fuzzySearch(id, inputId, listId, data, input.value);
        displayFuzzySearchResult(list, el.getBoundingClientRect());
    });
    input.addEventListener("blur", function () {
        // Delay blur action so that the list can handle click action before
        // blured out.
        setTimeout(function () {
            list.classList.remove("active-fuzzy-search");
        }, 120);
    });
    input.addEventListener("keypress", function () {
        var inputText = input.value;
    });

    registerFuzzySearchHandler(id, inputId, listId, data);
}

function registerJobResultEventHandler(li, inputId) {
    var input = document.getElementById(inputId);
    if (!input) {
        return;
    }

    li.addEventListener("click", function (event) {
        input.value = event.currentTarget.innerHTML;
        redraw();
    });
    li.addEventListener("mouseover", function (event) {
        var selectedJobs = document.getElementsByClassName("job-selected");
        if (!selectedJobs) {
            return;
        }

        for (var i = 0; i < selectedJobs.length; i++) {
            selectedJobs[i].classList.remove("job-selected");
        }
        event.currentTarget.classList.add("job-selected");
    });
    li.addEventListener("mouseout", function (event) {
        event.currentTarget.classList.remove("job-selected");
    });
}

function addOptionFuzzySearch(data, id, inputId, listId, stopAutoFill) {
    var input = document.getElementById(inputId);
    var list = document.getElementById(listId);

    if (!input || !list) {
        return;
    }
    if (!stopAutoFill) {
        input.value = getParameterByName(id);
    }
    while (list.firstChild) {
        list.removeChild(list.firstChild);
    }
    for (var i = 0; i < data.length; i++) {
        var li = document.createElement("li");
        li.innerHTML = data[i];
        registerJobResultEventHandler(li, inputId);
        list.appendChild(li);
    }
}

function addOptions(s, p) {
    var sel = document.getElementById(p);
    while (sel.length > 1) {
        sel.removeChild(sel.lastChild);
    }
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

function selectionText(sel, t) {
    return sel.selectedIndex == 0 ? "" : sel.options[sel.selectedIndex].text;
}

function equalSelected(sel, t) {
    return sel === "" || sel == t;
}

function groupKey(build) {
    return build.repo + " " + build.number + " " + build.refs;
}

function redraw() {
    var modal = document.getElementById('rerun');
    var rerun_command = document.getElementById('rerun-content');
    window.onclick = function (event) {
        if (event.target == modal) {
            modal.style.display = "none";
        }
    };
    var builds = document.getElementById("builds").getElementsByTagName(
        "tbody")[0];
    while (builds.firstChild) {
        builds.removeChild(builds.firstChild);
    }

    var args = [];

    function getSelection(name) {
        var sel = selectionText(document.getElementById(name));
        if (sel && opts && !opts[name + 's'][sel]) {
            return "";
        }
        if (sel !== "") {
            args.push(name + "=" + encodeURIComponent(sel));
        }
        return sel;
    }

    function getSelectionFuzzySearch(id, inputId) {
        var input = document.getElementById(inputId);
        if (!input) {
            return;
        }

        var inputText = input.value;
        if (inputText !== "" && opts && !opts[id + 's'][inputText]) {
            return "";
        }
        if (inputText !== "") {
            args.push(id + "=" + encodeURIComponent(
                inputText));
        }

        return inputText;
    }

    var opts = null;
    var repoSel = getSelection("repo");
    opts = optionsForRepo(repoSel);

    var typeSel = getSelection("type");
    var pullSel = getSelection("pull");
    var authorSel = getSelection("author");
    var jobSel = getSelectionFuzzySearch("job", "job-input");
    var stateSel = getSelection("state");

    if (window.history && window.history.replaceState !== undefined) {
        if (args.length > 0) {
            history.replaceState(null, "", "/?" + args.join('&'));
        } else {
            history.replaceState(null, "", "/")
        }
    }
    redrawOptions(opts);

    var lastKey = '';
    for (var i = 0, emitted = 0; i < allBuilds.length && emitted < 500; i++) {
        var build = allBuilds[i];
        if (!equalSelected(typeSel, build.type)) {
            continue;
        }
        if (!equalSelected(repoSel, build.repo)) {
            continue;
        }
        if (!equalSelected(stateSel, build.state)) {
            continue;
        }
        if (!equalSelected(jobSel, build.job)) {
            continue;
        }
        if (build.type === "presubmit") {
            if (!equalSelected(pullSel, build.number)) {
                continue;
            }
            if (!equalSelected(authorSel, build.author)) {
                continue;
            }
        } else if (pullSel || authorSel) {
            continue;
        }
        emitted++;

        var r = document.createElement("tr");
        r.appendChild(stateCell(build.state));
        if (build.pod_name) {
            r.appendChild(
                createLinkCell("\u2261", "log?job=" + build.job + "&id="
                    + build.build_id, "Build log."));
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
                r.appendChild(createLinkCell(build.repo, "https://github.com/"
                    + build.repo, ""));
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
    var url = "https://" + window.location.hostname + "/rerun?prowjob="
        + prowjob;
    var c = document.createElement("td");
    var a = document.createElement("a");
    a.href = "#";
    a.title = "Show instructions for rerunning this job.";
    a.onclick = function () {
        modal.style.display = "block";
        rerun_command.innerHTML = "kubectl create -f \"<a href='" + url + "'>"
            + url + "</a>\"";
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
    } else if (state === "failure" || state === "error" || state
        === "aborted") {
        c.appendChild(document.createTextNode("\u2717"));
    }
    return c;
}

function batchRevisionCell(build) {
    var c = document.createElement("td");
    var pr_refs = build.refs.split(",");
    for (var i = 1; i < pr_refs.length; i++) {
        if (i != 1) {
            c.appendChild(document.createTextNode(", "));
        }
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
    bl.href = "https://github.com/" + build.repo + "/commit/" + build.base_sha;
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
    cl.href = "https://github.com/" + build.repo + "/pull/" + build.number
        + '/commits/' + build.pull_sha;
    cl.text = build.pull_sha.slice(0, 7);
    c.appendChild(cl);
    c.appendChild(document.createTextNode(") by "));
    var al = document.createElement("a");
    al.href = "https://github.com/" + build.author;
    al.text = build.author;
    c.appendChild(al);
    return c;
}
