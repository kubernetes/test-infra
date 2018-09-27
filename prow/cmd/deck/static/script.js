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

function shortenBuildRefs(buildRef) {
    return buildRef && buildRef.replace(/:[0-9a-f]*/g, '');
}

function optionsForRepo(repo) {
    var opts = {
        types: {},
        repos: {},
        jobs: {},
        authors: {},
        pulls: {},
        batches: {},
        states: {},
    };

    for (var i = 0; i < allBuilds.length; i++) {
        var build = allBuilds[i];
        opts.types[build.type] = true;
        opts.repos[build.repo] = true;
        if (!repo || repo === build.repo) {
            opts.jobs[build.job] = true;
            opts.states[build.state] = true;
            if (build.type === "presubmit") {
                opts.authors[build.author] = true;
                opts.pulls[build.number] = true;
            } else if (build.type === "batch") {
                opts.batches[shortenBuildRefs(build.refs)] = true;
            }
        }
    }

    return opts;
}

function redrawOptions(fz, opts) {
    var ts = Object.keys(opts.types).sort();
    var selectedType = addOptions(ts, "type");
    var rs = Object.keys(opts.repos).filter(function (r) {
        return r !== "/";
    }).sort();
    addOptions(rs, "repo");
    var js = Object.keys(opts.jobs).sort();
    var jobInput = document.getElementById("job-input");
    var jobList = document.getElementById("job-list");
    addOptionFuzzySearch(fz, js, "job", jobList, jobInput);
    var as = Object.keys(opts.authors).sort(function (a, b) {
        return a.toLowerCase().localeCompare(b.toLowerCase());
    });
    addOptions(as, "author");
    if (selectedType === "batch") {
        opts.pulls = opts.batches;
    }
    if (selectedType !== "periodic" && selectedType !== "postsubmit") {
        var ps = Object.keys(opts.pulls).sort(function (a, b) {
            return parseInt(a) - parseInt(b);
        });
        addOptions(ps, "pull");
    } else {
        addOptions([], "pull");
    }
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
    var topNavigator = document.querySelector("#top-navigator");
    var navigatorTimeOut;
    var main = document.querySelector("main");
    main.onscroll = () => {
        topNavigator.classList.add("hidden");
        if (navigatorTimeOut) {
            clearTimeout(navigatorTimeOut);
        }
        navigatorTimeOut = setTimeout(() => {
            if (main.scrollTop === 0) {
                topNavigator.classList.add("hidden");
            } else if (main.scrollTop > 100) {
                topNavigator.classList.remove("hidden");
            }
        }, 100);
    };
    topNavigator.onclick = () => {
      main.scrollTop = 0;
    };

    document.addEventListener("keydown", function (event) {
        if (event.keyCode === 40) {
            handleDownKey();
        } else if (event.keyCode === 38) {
            handleUpKey();
        }
    });
    // Register selection on change functions
    var filterBox = document.querySelector("#filter-box");
    var options = filterBox.querySelectorAll("select");
    options.forEach(opt => {
        opt.onchange = () => {
            redraw(fz);
        };
    });
    // Attach job status bar on click
    var stateFilter = document.querySelector("#state");
    document.querySelectorAll(".job-bar-state").forEach(jb => {
        var state = jb.id.slice("job-bar-".length);
        if (state === "unknown") {
            return;
        }
        jb.addEventListener("click", () => {
            stateFilter.value = state;
            stateFilter.onchange();
        });
    });
    // set dropdown based on options from query string
    var opts = optionsForRepo("");
    var fz = initFuzzySearch(
        "job",
        "job-input",
        "job-list",
        Object.keys(opts["jobs"]).sort());
    redrawOptions(fz, opts);
    redraw(fz);
};

function displayFuzzySearchResult(el, inputContainer) {
    el.classList.add("active-fuzzy-search");
    el.style.top = inputContainer.height - 1 + "px";
    el.style.width = inputContainer.width + "px";
    el.style.height = 200 + "px";
    el.style.zIndex = "9999"
}

function fuzzySearch(fz, id, list, input) {
    var inputValue = input.value.trim();
    addOptionFuzzySearch(fz, fz.search(inputValue), id, list, input, true);
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

function handleEnterKeyDown(fz, list, input) {
    var selectedJobs = list.getElementsByClassName("job-selected");
    if (selectedJobs && selectedJobs.length === 1) {
        input.value = selectedJobs[0].innerHTML;
    }
    // TODO(@qhuynh96): according to discussion in https://github.com/kubernetes/test-infra/pull/7165, the
    // fuzzy search should respect user input no matter it is in the list or not. User may
    // experience being redirected back to default view if the search input is invalid.
    input.blur();
    list.classList.remove("active-fuzzy-search");
    redraw(fz);
}

function registerFuzzySearchHandler(fz, id, list, input) {
    input.addEventListener("keydown", function (event) {
        if (event.keyCode === 13) {
            handleEnterKeyDown(fz, list, input);
        } else if (validToken(event.keyCode)) {
            // Delay 1 frame that the input character is recorded before getting
            // input value
            setTimeout(function () {
                fuzzySearch(fz, id, list, input);
            }, 32);
        }
    });
}

function initFuzzySearch(id, inputId, listId, data) {
    var fz = new FuzzySearch(data);
    var el = document.getElementById(id);
    var input = document.getElementById(inputId);
    var list = document.getElementById(listId);

    list.classList.remove("active-fuzzy-search");
    input.addEventListener("focus", function () {
        fuzzySearch(fz, id, list, input);
        displayFuzzySearchResult(list, el.getBoundingClientRect());
    });
    input.addEventListener("blur", function () {
        list.classList.remove("active-fuzzy-search");
    });

    registerFuzzySearchHandler(fz, id, list, input);
    return fz;
}

function registerJobResultEventHandler(fz, li, input) {
    li.addEventListener("mousedown", function (event) {
        input.value = event.currentTarget.innerHTML;
        redraw(fz);
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

function addOptionFuzzySearch(fz, data, id, list, input, stopAutoFill) {
    if (!stopAutoFill) {
        input.value = getParameterByName(id);
    }
    while (list.firstChild) {
        list.removeChild(list.firstChild);
    }
    list.scrollTop = 0;
    for (var i = 0; i < data.length; i++) {
        var li = document.createElement("li");
        li.innerHTML = data[i];
        registerJobResultEventHandler(fz, li, input);
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
    return param;
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

function redraw(fz) {
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
    if (typeSel === "batch") {
        opts.pulls = opts.batches;
    }
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
    fz.setDict(Object.keys(opts.jobs));
    redrawOptions(fz, opts);

    var lastKey = '';
    const jobCountMap = new Map();
    let totalJob = 0;
    for (var i = 0; i < allBuilds.length; i++) {
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
        } else if (build.type === "batch" && !authorSel) {
            if (!equalSelected(pullSel, shortenBuildRefs(build.refs))) {
                continue;
            }
        } else if (pullSel || authorSel) {
            continue;
        }

        if (!jobCountMap.has(build.state)) {
          jobCountMap.set(build.state, 0);
        }
        totalJob ++;
        jobCountMap.set(build.state, jobCountMap.get(build.state) + 1);
        if (totalJob > 499) {
            continue;
        }
        var r = document.createElement("tr");
        r.appendChild(stateCell(build.state));
        if (build.pod_name) {
            const icon = createIcon("description", "Build log");
            icon.href = "log?job=" + build.job + "&id=" + build.build_id;
            const cell = document.createElement("TD");
            cell.classList.add("icon-cell");
            cell.appendChild(icon);
            r.appendChild(cell);
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
            } else if (build.repo.startsWith("http://") || build.repo.startsWith("https://") ) {
                r.appendChild(createLinkCell(build.repo, build.repo, ""));
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
        if (spyglass) {
            if (build.state == "pending") {
                let url = window.location.origin + "/view/prowjob/" + build.prow_job;
                r.appendChild(createSpyglassCell(url));
            } else {
                const buildIndex = build.url.indexOf("/build/");
                if (buildIndex !== -1) {
                    let url = window.location.origin + "/view/gcs/" +
                        build.url.substring(buildIndex + "/build/".length);
                    r.appendChild(createSpyglassCell(url));
                } else {
                    r.appendChild(createTextCell(""));
                }
            }
        } else {
            r.appendChild(createTextCell(""));
        }
        if (build.url === "") {
            r.appendChild(createTextCell(build.job));
        } else {
            r.appendChild(createLinkCell(build.job, build.url, ""));
        }

        r.appendChild(createTimeCell(i, parseInt(build.started)));
        r.appendChild(createTextCell(build.duration));
        builds.appendChild(r);
    }
    const jobCount = document.getElementById("job-count");
    jobCount.textContent = "Showing " + Math.min(totalJob, 500) + "/" + totalJob + " jobs";
    drawJobBar(totalJob, jobCountMap);
}

function createSpyglassCell(url) {
    const icon = createIcon("visibility", "View in Spyglass");
    icon.href = url;
    const cell = document.createElement("TD");
    cell.classList.add("icon-cell");
    cell.appendChild(icon);
    return cell;
}

function createTextCell(text) {
    var c = document.createElement("td");
    c.appendChild(document.createTextNode(text));
    return c;
}

function createTimeCell(id, time) {
    var momentTime = moment.unix(time);
    var tid = "time-cell-" + id;
    var main = document.createElement("div");
    var isADayOld = momentTime.isBefore(moment().startOf('day'));
    main.textContent = momentTime.format(isADayOld ? 'MMM DD HH:mm:ss' : 'HH:mm:ss');
    main.id = tid;

    var tooltip = document.createElement("div");
    tooltip.textContent = momentTime.format('MMM DD YYYY, HH:mm:ss [UTC]ZZ');
    tooltip.setAttribute("data-mdl-for", tid);
    tooltip.classList.add("mdl-tooltip", "mdl-tooltip--large");

    var c = document.createElement("td");
    c.appendChild(main);
    c.appendChild(tooltip);

    return c;
}

function createLinkCell(text, url, title) {
    const c = document.createElement("td");
    const a = document.createElement("a");
    a.href = url;
    if (title !== "") {
        a.title = title;
    }
    a.appendChild(document.createTextNode(text));
    c.appendChild(a);
    return c;
}

function createRerunCell(modal, rerun_command, prowjob) {
    const url = "https://" + window.location.hostname + "/rerun?prowjob="
        + prowjob;
    const c = document.createElement("td");
    const icon = createIcon("refresh", "Show instructions for rerunning this job");
    icon.onclick = function () {
        modal.style.display = "block";
        const rerun_html = "kubectl create -f \"<a href='" + url + "'>"
        + url + "</a>\" " 
        + "<a class='mdl-button mdl-js-button mdl-button--icon' onclick=\""+
        "copyToClipboardWithToast('kubectl create -f " + url + "')\">"
        + "<i class='material-icons state triggered' style='color: gray'>file_copy</i></a>";
        rerun_command.innerHTML = rerun_html;
    };
    c.appendChild(icon);
    c.classList.add("icon-cell");
    return c;
}

// copyToClipboard is from https://stackoverflow.com/a/33928558
// Copies a string to the clipboard. Must be called from within an 
// event handler such as click. May return false if it failed, but
// this is not always possible. Browser support for Chrome 43+, 
// Firefox 42+, Safari 10+, Edge and IE 10+.
// IE: The clipboard feature may be disabled by an administrator. By
// default a prompt is shown the first time the clipboard is 
// used (per session).
function copyToClipboard(text) {
    if (window.clipboardData && window.clipboardData.setData) {
        // IE specific code path to prevent textarea being shown while dialog is visible.
        return clipboardData.setData("Text", text); 

    } else if (document.queryCommandSupported && document.queryCommandSupported("copy")) {
        var textarea = document.createElement("textarea");
        textarea.textContent = text;
        textarea.style.position = "fixed";  // Prevent scrolling to bottom of page in MS Edge.
        document.body.appendChild(textarea);
        textarea.select();
        try {
            return document.execCommand("copy");  // Security exception may be thrown by some browsers.
        } catch (ex) {
            console.warn("Copy to clipboard failed.", ex);
            return false;
        } finally {
            document.body.removeChild(textarea);
        }
    }
}

function copyToClipboardWithToast(text) {
    copyToClipboard(text);
    const toast = document.body.querySelector("#toast");
    toast.MaterialSnackbar.showSnackbar({message: "Copied to clipboard"});
}

function stateCell(state) {
    const c = document.createElement("td");
    if (!state || state === "") {
        c.appendChild(document.createTextNode(""));
        return c;
    }
    c.classList.add("icon-cell");

    let displayState = stateToAdj(state);
    displayState = displayState[0].toUpperCase() + displayState.slice(1);
    let displayIcon = "";
    switch (state) {
        case "triggered":
            displayIcon = "schedule";
            break;
        case "pending":
            displayIcon = "watch_later";
            break;
        case "success":
            displayIcon = "check_circle";
            break;
        case "failure":
            displayIcon = "error";
            break;
        case "aborted":
            displayIcon = "remove_circle";
            break;
        case "error":
            displayIcon = "warning";
            break;
    }
    const stateIndicator = document.createElement("I");
    stateIndicator.classList.add("material-icons", "state", state);
    stateIndicator.innerText = displayIcon;
    c.appendChild(stateIndicator);
    c.title = displayState;

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

function drawJobBar(total, jobCountMap) {
  const states = ["success", "pending", "triggered", "error", "failure", "aborted", ""];
  states.sort((s1, s2) => {
    return jobCountMap.get(s1) - jobCountMap.get(s2);
  });
  states.forEach((state, index) => {
    const count = jobCountMap.get(state);
    // If state is undefined or empty, treats it as unkown state.
    if (!state || state === "") {
      state = "unknown";
    }
    const id = "job-bar-" + state;
    const el = document.getElementById(id);
    const tt = document.getElementById(state + "-tooltip");
    if (!count || count === 0 || total === 0) {
      el.textContent = "";
      tt.textContent = "";
      el.style.width = "0";
    } else {
      el.textContent = count;
      tt.textContent = count + " " + stateToAdj(state) + " jobs";
      if (index === states.size - 1) {
        el.style.width = "auto";
      } else {
        el.style.width = Math.max((count / total * 100), 1) + "%";
      }
    }
  });
}

function stateToAdj(state) {
    switch (state) {
        case "success":
            return "succeeded";
        case "failure":
            return "failed";
        default:
            return state;
    }
}

/**
 * Returns an icon element.
 * @param {string} iconString icon name
 * @param {string} tooltip tooltip string
 * @return {Element}
 */
function createIcon(iconString, tooltip = "") {
    const icon = document.createElement("I");
    icon.classList.add(...["icon-button", "material-icons"]);
    icon.innerHTML = iconString;
    if (tooltip !== "") {
        icon.title = tooltip;
    }

    const container = document.createElement("A");
    container.appendChild(icon);
    container.classList.add(...["mdl-button", "mdl-js-button", "mdl-button--icon"]);

    return container;
}
