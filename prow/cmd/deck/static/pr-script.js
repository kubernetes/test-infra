'use strict';

/**
 * Submit the query by redirecting the page with the query and let window.onload
 * sends the request.
 * @param {Element} input query input element
 */
function submitQuery(input) {
    const query = getPRQuery(input.value);
    input.value = query;
    window.location.search = '?query=' + encodeURIComponent(query);
}

/**
 * Creates a XMLHTTP request to /pr-data.js.
 * @param {function} fulfillFn
 * @param {function} errorHandler
 * @return {XMLHttpRequest}
 */
function createXMLHTTPRequest(fulfillFn, errorHandler) {
    const request = new XMLHttpRequest();
    const url = "/pr-data.js";
    request.onreadystatechange = () => {
        if (request.readyState === 4 && request.status === 200) {
            fulfillFn(request);
        }
    };
    request.onerror = () => {
        errorHandler();
    };
    request.withCredentials = true;
    request.open("POST", url, true);
    request.setRequestHeader("Content-type", "application/x-www-form-urlencoded");

    return request;
}

/**
 * Makes sure the search query is looking for pull requests by dropping all
 * is:issue and type:pr tokens and adds is:pr if does not exist.
 * @param {string} q
 * @return {string}
 */
function getPRQuery(q) {
    const tokens = q.replace(/\+/g, " ").split(" ");
    // Firstly, drop all pr/issue search tokens
    let result = tokens.filter(tkn => {
        tkn = tkn.trim();
        return !(tkn === "is:issue" || tkn === "type:issue" || tkn === "is:pr"
            || tkn === "type:pr");
    }).join(" ");
    // Returns the query with is:pr to the start of the query
    result = "is:pr " + result;
    return result;
}

/**
 * Redraw the page
 * @param {Object} prData
 */
function redraw(prData) {
    const mainContainer = document.querySelector("#main-container");
    while (mainContainer.firstChild) {
        mainContainer.removeChild(mainContainer.firstChild);
    }
    if (prData && prData.Login) {
        loadPrStatus(prData);
    } else {
        loadGithubLogin();
    }
}

/**
 * Enables/disables the progress bar.
 * @param {boolean} isStarted
 */
function loadProgress(isStarted) {
    const pg = document.querySelector("#loading-progress");
    if (isStarted) {
        pg.classList.remove("hidden");
    } else {
        pg.classList.add("hidden");
    }
}

/**
 * Handles the URL query on load event.
 */
function onLoadQuery() {
    const query = window.location.search.substring(1);
    const params = query.split("&");
    if (!params[0]) {
        return "";
    }
    const val = params[0].slice("query=".length);
    if (val && val !== "") {
        return decodeURIComponent(val.replace(/\+/g, ' '));
    }
    return "";
}

/**
 * Gets cookie by its name.
 * @param {string} name
 */
function getCookieByName(name) {
    if (!document.cookie) {
        return "";
    }
    const cookies = decodeURIComponent(document.cookie).split(";");
    for (let i = 0; i < cookies.length; i++) {
        const c = cookies[i].trim();
        const pref = name + "=";
        if (c.indexOf(pref) === 0) {
            return c.slice(pref.length);
        }
    }
    return "";
}

window.onload = () => {
    document.querySelectorAll("dialog").forEach((dialog) => {
        dialogPolyfill.registerDialog(dialog);
        dialog.querySelector('.close').addEventListener('click', () => {
            dialog.close();
        });
    });
    // Check URL, if the search is empty, adds search query by default format
    // ?is:pr state:open query="author:<user_login>"
    if (window.location.search === "") {
        const login = getCookieByName("github_login");
        const searchQuery = "is:pr state:open " + "author:" + login;
        window.location.search = "?query=" + encodeURIComponent(searchQuery);
    }
    const request = createXMLHTTPRequest((request) => {
        const prData = JSON.parse(request.responseText);
        redraw(prData);
        loadProgress(false);
    }, () => {
        loadProgress(false);
        const mainContainer = document.querySelector("#main-container");
            mainContainer.appendChild(createMessage("Something wrongs! We could not fulfill your request"));
    });
    loadProgress(true);
    request.send("query=" + onLoadQuery());
};

function createSearchCard() {
    const searchCard = document.createElement("DIV");
    searchCard.id = "search-card";
    searchCard.classList.add("pr-card", "mdl-card");

    // Input box
    const input = document.createElement("TEXTAREA");
    input.id = "search-input";
    input.value = decodeURIComponent(window.location.search.slice("query=".length + 1));
    input.rows = 1;
    input.spellcheck = false;
    input.addEventListener("keydown", (event) => {
        if (event.keyCode === 13) {
            event.preventDefault();
            submitQuery(input);
        } else {
            const el = event.target;
            el.style.height  = "auto";
            el.style.height = event.target.scrollHeight + "px";
        }
    });
    input.addEventListener("focus", (event) => {
        const el = event.target;
        el.style.height  = "auto";
        el.style.height = event.target.scrollHeight + "px";
    });
    // Refresh button
    const refBtn = createIcon("refresh", "Reload the query", ["search-button"], true);
    refBtn.addEventListener("click", () => {
        submitQuery(input);
    }, true);
    const userBtn = createIcon("person", "Show my open pull requests", ["search-button"], true);
    userBtn.addEventListener("click", () => {
        const login = getCookieByName("github_login");
        const searchQuery = "is:pr state:open " + "author:" + login;
        window.location.search = "?query=" + encodeURIComponent(searchQuery);
    });

    const actionCtn = document.createElement("DIV");
    actionCtn.id = "search-action";
    actionCtn.appendChild(userBtn);
    actionCtn.appendChild(refBtn);

    const inputContainer = document.createElement("DIV");
    inputContainer.id = "search-input-ctn";
    inputContainer.appendChild(input);
    inputContainer.appendChild(actionCtn);

    const title = document.createElement("H6");
    title.textContent = "Github search query";
    const infoBtn = createIcon("info", "More information about the search query", ["search-info"], true);
    const titleCtn = document.createElement("DIV");
    titleCtn.appendChild(title);
    titleCtn.appendChild(infoBtn);
    titleCtn.classList.add("search-title");

    const searchDialog = document.querySelector("#search-dialog");
    infoBtn.addEventListener("click", () => {
        searchDialog.showModal();
    });

    searchCard.appendChild(titleCtn);
    searchCard.appendChild(inputContainer);
    return searchCard;
}

/**
 * Loads Pr Status
 */
function loadPrStatus(prData) {
    const repoTideQueryMap = new Map();
    for (let query of tideData.TideQueries) {
        for (let repo of query.repos) {
            if (!repoTideQueryMap[repo]) {
                repoTideQueryMap[repo] = [];
            }
            repoTideQueryMap[repo].push({
                labels: query.labels,
                missingLabels: query.missingLabels
            });
        }
    }
    const container = document.querySelector("#main-container");
    container.appendChild(createSearchCard());
    if (!prData.PullRequests || prData.PullRequests.length === 0) {
        const msg = createMessage("No open PRs found", "");
        container.appendChild(msg);
        return;
    }
    for (let pr of prData.PullRequests) {
        // There might be multiple runs of jobs for a build.
        // allBuilds is sorted with the most recent builds first, so
        // we only need to keep the first build for each job name.
        let seenJobs = {};
        let builds = [];
        for (let build of allBuilds) {
            if (build.type === 'presubmit' &&
                build.repo === pr.Repository.NameWithOwner &&
                build.base_ref === pr.BaseRef.Name &&
                build.number === pr.Number &&
                build.pull_sha === pr.HeadRefOID) {
                if (!seenJobs[build.job]) {  // First (latest) build for job.
                    seenJobs[build.job] = true;
                    builds.push(build);
                }
            }
        }
        container.appendChild(createPRCard(pr, builds,
            repoTideQueryMap[pr.Repository.NameWithOwner], tideData.Pools));
    }
}

/**
 * Creates Pool labels.
 * @param pr
 * @param tidePool
 * @return {Element}
 */
function createTidePoolLabel(pr, tidePool) {
    if (!tidePool || tidePool.length === 0) {
        return null;
    }
    const poolTypes = [tidePool.Target, tidePool.BatchPending,
        tidePool.SuccessPRs, tidePool.PendingPRs, tidePool.MissingPRs];
    const inPoolId = poolTypes.findIndex(poolType => {
        if (!poolType) {
            return false;
        }
        const index = poolType.findIndex(prInPool => {
            return prInPool.Number === pr.Number;
        });
        return index !== -1;
    });
    const label = document.createElement("SPAN");
    if (inPoolId === -1) {
        return null;
    }
    const labelTitle = ["Merging", "In Batch & Test Pending",
        "Test Passing & Merge Pending", "Test Pending",
        "Test failed/Missing Labels"];
    const labelStyle = ["merging", "batching", "passing", "pending", "failed"];
    label.textContent = "In Pool - " + labelTitle[inPoolId];
    label.classList.add("title-label", "mdl-shadow--2dp", labelStyle[inPoolId]);

    return label;
}

/**
 * Creates a label for the title. It will prioritise the merge status over the
 * job status. Saying that, if the pr has jobs failed and does not meet merge
 * requirements, it will show that the PR needs to resolve labels.
 * @param isMerge {boolean}
 * @param jobStatus {string}
 * @param mergeAbility {number}
 * @return {Element}
 */
function createTitleLabel(isMerge, jobStatus, mergeAbility) {
    const label = document.createElement("SPAN");
    label.classList.add("title-label", "mdl-shadow--2dp");
    if (isMerge) {
        label.textContent = "Merged";
        label.classList.add("merging");
    } else if (mergeAbility === -1) {
        label.textContent = "Unknown Merge Requirements";
        label.classList.add("unknown");
    } else if (mergeAbility === 0) {
        label.textContent = "Needs to Resolve Labels";
        label.classList.add("pending");
    } else {
        if (jobStatus === "succeeded") {
            label.textContent = "Good to be merged";
            label.classList.add(jobStatus);
        } else {
            label.textContent = "Jobs " + jobStatus;
            label.classList.add(jobStatus);
        }
    }

    return label;
}

/**
 * Creates PR Card title.
 * @param {Object} pr
 * @param {Array<Object>} tidePools
 * @param {string} jobStatus
 * @param {number} mergeAbility
 * @return {Element}
 */
function createPRCardTitle(pr, tidePools, jobStatus, mergeAbility) {
    const prTitle = document.createElement("DIV");
    prTitle.classList.add("mdl-card__title");

    const title = document.createElement("H4");
    title.textContent = "#" + pr.Number;
    title.classList.add("mdl-card__title-text");

    const subtitle = document.createElement("H5");
    subtitle.textContent = pr.Repository.NameWithOwner;
    subtitle.classList.add("mdl-card__subtitle-text");

    const link = document.createElement("A");
    link.href = "https://github.com/" + pr.Repository.NameWithOwner + "/pull/"
        + pr.Number;
    link.appendChild(title);

    const prTitleText = document.createElement("DIV");
    prTitleText.appendChild(link);
    prTitleText.appendChild(subtitle);
    prTitleText.classList.add("pr-title-text");
    prTitle.appendChild(prTitleText);

    const pool = tidePools.filter(pool => {
        const repo = pool.Org + "/" + pool.Repo;
        return pr.Repository.NameWithOwner === repo && pr.BaseRef.Name
            === pool.Branch;
    });
    let tidePoolLabel = createTidePoolLabel(pr, pool[0]);
    if (!tidePoolLabel) {
        tidePoolLabel = createTitleLabel(pr.Merged, jobStatus, mergeAbility);
    }
    prTitle.appendChild(tidePoolLabel);

    return prTitle;
}

/**
 * Creates a list of jobs.
 * @param list
 * @param itemStyle
 * @return {Element}
 */
function createList(list, itemStyle = []) {
    const container = document.createElement("UL");
    container.classList.add("mdl-list", "job-list");
    const getStateIcon = (state) => {
        switch (state) {
            case "success":
                return "check_circle";
            case "failure":
                return "error";
            case "pending":
                return "watch_later";
            case "triggered":
                return "schedule";
            case "aborted":
                return "remove_circle";
            case "error":
                return "warning";
            default:
                return "";
        }
    };
    list.forEach(el => {
        const elCon = document.createElement("LI");
        elCon.classList.add("mdl-list__item", "job-list-item", ...itemStyle);
        const item = document.createElement("A");
        item.classList.add("mdl-list__item-primary-content");
        item.href = el.url;
        const icon = document.createElement("I");
        icon.textContent = getStateIcon(el.state);
        icon.classList.add("state", el.state, "material-icons", "mdl-list__item-icon");
        item.appendChild(icon);
        item.appendChild(document.createTextNode(el.context));
        elCon.appendChild(item);
        container.appendChild(elCon);
    });
    return container;
}

/**
 * Creates Job status.
 * @param builds
 * @return {Element}
 */
function createJobStatus(builds) {
    const statusContainer = document.createElement("DIV");
    statusContainer.classList.add("status-container");
    const status = document.createElement("DIV");
    const failedJobs = builds.filter(build => {
        return build.state === "failure";
    });
    // Job status indicator
    const state = jobStatus(builds);
    let statusText = "";
    let stateIcon = "";
    switch (state) {
        case "succeeded":
            statusText = "All tests passed";
            stateIcon = "check_circle";
            break;
        case "failed":
            statusText = failedJobs.length + " test" + (failedJobs.length === 1 ? "" : "s") + " failed";
            stateIcon = "error";
            break;
        case "unknown":
            statusText = "No test found";
            break;
        default:
            statusText = "Tests are running";
            stateIcon = "watch_later";
    }
    const arrowIcon = createIcon("expand_more");
    arrowIcon.classList.add("arrow-icon");
    if (state === "unknown") {
        arrowIcon.classList.add("hidden");
        const p = document.createElement("P");
        p.textContent = "Test results for this PR are not in our record but you can always find them on PR's GitHub page. Sorry for any inconvenience!";

        status.appendChild(document.createTextNode(statusText));
        status.appendChild(createStatusHelp("No test found", [p]));
        status.classList.add("no-status");
    } else {
        status.appendChild(createIcon(stateIcon, "", ["status-icon", state]));
        status.appendChild(document.createTextNode(statusText));
    }
    status.classList.add("status");
    statusContainer.appendChild(status);
    // Job list
    let failedJobsList;
    if (failedJobs.length > 0) {
        failedJobsList = createList(failedJobs);
        statusContainer.appendChild(failedJobsList);
    }
    const jobList = createList(builds);
    jobList.classList.add("hidden");
    status.addEventListener("click", () => {
        if (state === "unknown") {
            return;
        }
        if (failedJobsList) {
            failedJobsList.classList.add("hidden");
        }
        jobList.classList.toggle("hidden");
        arrowIcon.textContent = arrowIcon.textContent === "expand_more"
            ? "expand_less" : "expand_more";
    });

    status.appendChild(arrowIcon);
    statusContainer.appendChild(jobList);
    return statusContainer;
}

/**
 * Creates a merge requirement cell.
 * @param labels
 * @param notMissingLabel
 * @return {Element}
 */
function createMergeLabelCell(labels, notMissingLabel = false) {
    const cell = document.createElement("TD");
    labels.forEach(label => {
        const labelEl = document.createElement("SPAN");
        const name = label.name.split(" ").join("");
        labelEl.classList.add("merge-table-label", "mdl-shadow--2dp", "label",
            name);
        labelEl.textContent = label.name;
        const toDisplay = label.own ^ notMissingLabel;
        if (toDisplay) {
            cell.appendChild(labelEl);
        }
    });

    return cell;
}

/**
 * Appends labels to a container
 * @param {Element} container
 * @param {Array<string>} labels
 */
function appendLabelsToContainer(container, labels) {
    while (container.firstChild) {
        container.removeChild(container.firstChild);
    }
    labels.forEach(label => {
        const labelEl = document.createElement("SPAN");
        const labelName = label.split(" ").join("");
        labelEl.classList.add("merge-table-label", "mdl-shadow--2dp", "label", labelName);
        labelEl.textContent = label;
        container.appendChild(labelEl);
    });
}

/**
 * Creates merge requirement table for queries.
 * @param prLabels
 * @param queries
 * @return {Element}
 */
function createQueriesTable(prLabels, queries) {
    const table = document.createElement("TABLE");
    table.classList.add("merge-table");
    const thead = document.createElement("THEAD");
    const allLabelHeaderRow = document.createElement("TR");
    const allLabelHeaderCell = document.createElement("TD");
    // Creates all pr labels header.
    allLabelHeaderCell.textContent = "PR's Labels";
    allLabelHeaderCell.colSpan = 3;
    allLabelHeaderRow.appendChild(allLabelHeaderCell);
    thead.appendChild(allLabelHeaderRow);

    const allLabelRow = document.createElement("TR");
    const allLabelCell = document.createElement("TD");
    allLabelCell.colSpan = 3;
    appendLabelsToContainer(allLabelCell, prLabels.map(label => {
        return label.Label.Name;
    }));
    allLabelRow.appendChild(allLabelCell);
    thead.appendChild(allLabelRow);

    const tableRow = document.createElement("TR");
    const col1 = document.createElement("TD");
    col1.textContent = "Required Labels (Missing)";
    const col2 = document.createElement("TD");
    col2.textContent = "Forbidden Labels (Shouldn't have)";
    const col3 = document.createElement("TD");

    const body = document.createElement("TBODY");
    queries.forEach(query => {
        const row = document.createElement("TR");
        row.append(createMergeLabelCell(query.labels, true));
        row.append(createMergeLabelCell(query.missingLabels));

        const mergeIcon = document.createElement("TD");
        mergeIcon.classList.add("merge-table-icon");
        const iconButton = createIcon("information", "Clicks to see query details", [], true);
        mergeIcon.appendChild(iconButton);
        row.appendChild(mergeIcon);

        body.appendChild(row);
        const dialog = document.querySelector("#query-dialog");
        const allRequired = document.querySelector("#query-all-required");
        const allForbidden = document.querySelector("#query-all-forbidden");
        iconButton.addEventListener("click", () => {
            appendLabelsToContainer(allRequired, query.labels.map(label => {
                return label.name;
            }));
            appendLabelsToContainer(allForbidden, query.missingLabels.map(label => {
                return label.name;
            }));
            dialog.showModal();
        });
    });

    tableRow.appendChild(col1);
    tableRow.appendChild(col2);
    tableRow.appendChild(col3);
    thead.appendChild(tableRow);
    table.appendChild(thead);
    table.appendChild(body);

    return table;
}

/**
 * Creates the merge requirement status.b
 * @param prLabels
 * @param queries
 * @return {Element}
 */
function createMergeStatus(prLabels, queries) {
    prLabels = prLabels ? prLabels : [];
    const statusContainer = document.createElement("DIV");
    statusContainer.classList.add("status-container");
    const status = document.createElement("DIV");
    const mergeAbility = isAbleToMerge(queries);
    if (mergeAbility === 0) {
        status.appendChild(createIcon("error", "", ["status-icon", "failed"]));
        status.appendChild(document.createTextNode("Does not meet merge requirements"));
        // Creates help button
        const iconButton = createIcon("help", "", ["help-icon-button"], true);
        status.appendChild(iconButton);
        // Shows dialog
        const dialog = document.querySelector("#merge-help-dialog");
        iconButton.addEventListener("click", (event) => {
            dialog.showModal();
            event.stopPropagation();
        });
    } else if (mergeAbility === 1) {
        status.appendChild(createIcon("check_circle", "", ["status-icon", "succeeded"]));
        status.appendChild(document.createTextNode("Meets merge requirements"));
    } else {
        status.appendChild(document.createTextNode("No Tide query found"));
        status.classList.add("no-status");
        const p = document.createElement("P");
        p.textContent = "This repo may not be configured to use Tide.";
        status.appendChild(createStatusHelp("Tide query not found", [p]));
    }
    const arrowIcon= createIcon("expand_less");
    arrowIcon.classList.add("arrow-icon");

    status.classList.add("status");
    status.appendChild(arrowIcon);

    const queriesTable = createQueriesTable(prLabels, queries);
    if (mergeAbility !== 0) {
        queriesTable.classList.add("hidden");
        arrowIcon.textContent = "expand_more";
    }
    status.addEventListener("click", () => {
        queriesTable.classList.toggle("hidden");
        if (queriesTable.classList.contains("hidden")) {
            const offLabels = queriesTable.querySelectorAll(
                ".merge-table-label.off");
            offLabels.forEach(offLabel => {
                offLabel.classList.add("hidden");
            });
        }
        arrowIcon.textContent = arrowIcon.textContent === "expand_more"
            ? "expand_less" : "expand_more";
    });

    statusContainer.appendChild(status);
    statusContainer.appendChild(queriesTable);
    return statusContainer;
}

/**
 * Creates a help button on the status.
 * @param {string} title
 * @param {Array<Element>} content
 * @return {Element}
 */
function createStatusHelp(title, content) {
    const dialog = document.querySelector("#status-help-dialog");
    const dialogTitle = dialog.querySelector(".mdl-dialog__title");
    const dialogContent = dialog.querySelector(".mdl-dialog__content");
    const helpIcon = createIcon("help", "", ["help-icon-button"], true);
    helpIcon.addEventListener("click", (event) => {
        dialogTitle.textContent = title;
        while (dialogContent.firstChild) {
            dialogContent.removeChild(dialogContent.firstChild);
        }
        content.forEach(el => {
            dialogContent.appendChild(el);
        });
        dialog.showModal();
        event.stopPropagation();
    });

    return helpIcon;
}

function createPRCardBody(pr, builds, queries) {
    const cardBody = document.createElement("DIV");
    const title = document.createElement("H3");
    title.textContent = pr.Title;

    cardBody.classList.add("mdl-card__supporting-text");
    cardBody.appendChild(title);
    cardBody.appendChild(createJobStatus(builds));
    const nodes = pr.Labels && pr.Labels.Nodes ? pr.Labels.Nodes : [];
    cardBody.appendChild(createMergeStatus(nodes, queries));

    return cardBody;
}

/**
 * Compare function that prioritizes jobs which are in failure state.
 * @param a
 * @param b
 * @return {number}
 */
function compareJobFn(a, b) {
    const stateToPrio = new Map();
    stateToPrio["success"] = 3;
    stateToPrio["aborted"] = 2;
    stateToPrio["pending"] = stateToPrio["triggered"] = 1;
    stateToPrio["error"] = stateToPrio["failure"] = 0;

    return stateToPrio[a.state] > stateToPrio[b.state] ? 1
        : stateToPrio[a.state] < stateToPrio[b.state] ? -1 : 0;
}

/**
 * Creates a PR card.
 * @param {Object} pr
 * @param {Array<Object>} builds
 * @param {Array<Object>} queries
 * @param {Array<Object>} tidePools
 * @return {Element}
 */
function createPRCard(pr, builds = [], queries = [], tidePools = []) {
    builds = builds ? builds : [];
    queries = queries ? queries : [];
    tidePools = tidePools ? tidePools : [];
    const prCard = document.createElement("DIV");
    // jobs need to be sorted from high priority (failure, error) to low
    // priority (success)
    builds.sort(compareJobFn);
    const prLabelsSet = new Set();
    if (pr.Labels && pr.Labels.Nodes) {
        pr.Labels.Nodes.forEach(label => {
            prLabelsSet.add(label.Label.Name);
        });
    }
    const processedQuery = [];
    queries.forEach(query => {
        let score = 0.0;
        const labels = [];
        const missingLabels = [];
        query.labels.sort((a, b) => {
            if (a.length === b.length) {
                return 0;
            }
            return a.length < b.length ? -1 : 1;
        });
        query.missingLabels.sort((a, b) => {
            if (a.length === b.length) {
                return 0;
            }
            return a.length < b.length ? -1 : 1;
        });
        query.labels.forEach(label => {
            labels.push({name: label, own: prLabelsSet.has(label)});
            score += labels[labels.length - 1].own ? 1 : 0;
        });
        query.missingLabels.forEach(label => {
            missingLabels.push({name: label, own: prLabelsSet.has(label)});
            score += missingLabels[missingLabels.length - 1].own ? 0 : 1;
        });
        score = (labels.length + missingLabels.length > 0) ? score
            / (labels.length + missingLabels.length) : 1.0;
        processedQuery.push(
            {score: score, labels: labels, missingLabels: missingLabels});
    });
    // Sort queries by descending score order.
    processedQuery.sort((q1, q2) => {
        if (Math.abs(q1.score - q2.score) < Number.EPSILON) {
            return 0;
        }
        return q1.score > q2.score ? -1 : 1;
    });
    prCard.classList.add("pr-card", "mdl-card");
    prCard.appendChild(createPRCardTitle(pr, tidePools, jobStatus(builds), isAbleToMerge(processedQuery)));
    prCard.appendChild(createPRCardBody(pr, builds, processedQuery));
    return prCard;
}

/**
 * Load Github login button if user has not login.
 */
function loadGithubLogin() {
    const button = document.createElement("BUTTON");
    button.classList.add("mdl-button", "mdl-js-button", "mdl-button--raised",
        "mdl-button--primary", "mdl-js-ripple-effect");
    button.textContent = "Login to Github";
    button.style.alignSelf = "center";
    button.style.width = "160px";
    button.addEventListener("click", () => {
        const url = window.location;
        window.location.href = url.origin + "/github-login";
    });
    const msg = createMessage(
        "PR Status needs you to login and grant it OAuth scopes",
        "sentiment_very_satisfied");
    const main = document.querySelector("#main-container");
    main.appendChild(msg);
    main.appendChild(button);
}

/**
 * Returns the job status based on its state.
 * @param builds
 * @return {string}
 */
function jobStatus(builds) {
    if (builds.length === 0) {
        return "unknown";
    }
    switch (builds[0].state) {
        case "success":
            return "succeeded";
        case "failure":
            return "failed";
        default:
            return "pending";
    }
}

/**
 * Returns -1 if there is no query. 1 if the PR is able to be merged by checking
 * the score of the first query in the query list (score === 1), the list has
 * been sorted by scores, otherwise 0.
 * @param queries
 * @return {number}
 */
function isAbleToMerge(queries) {
    if (queries.length === 0) {
        return -1;
    }
    return Math.abs(queries[0].score - 1.0) < Number.EPSILON ? 1 : 0;
}

/**
 * Returns an icon element.
 * @param {string} iconString icon name
 * @param {string} tooltip tooltip string
 * @param {Array<string>} styles
 * @param {boolean} isButton
 * @return {Element}
 */
function createIcon(iconString, tooltip = "", styles = [], isButton = false) {
    const icon = document.createElement("I");
    icon.classList.add("icon-button", "material-icons");
    icon.textContent = iconString;
    if (tooltip !== "") {
        icon.title = tooltip;
    }
    if (!isButton) {
        icon.classList.add(...styles);
        return icon;
    }
    const container = document.createElement("BUTTON");
    container.appendChild(icon);
    container.classList.add("mdl-button", "mdl-js-button", "mdl-button--icon",
        ...styles);

    return container;
}

/**
 * Create a simple message with an icon.
 * @param msg
 * @param icStr
 * @return {HTMLElement}
 */
function createMessage(msg, icStr) {
    const el = document.createElement("H3");
    el.textContent = msg;
    if (icStr !== "") {
        const ic = createIcon(icStr, "", ["message-icon"]);
        el.appendChild((ic));
    }
    const msgContainer = document.createElement("DIV");
    msgContainer.appendChild(el);
    msgContainer.classList.add("message");

    return msgContainer;
}

document.addEventListener("DOMContentLoaded", function(event) {
   configure();
});

function configure() {
    if(typeof branding === undefined){
        return;
    }
    if (branding.logo !== '') {
        document.getElementById('img').src = branding.logo;
    }
    if (branding.favicon !== '') {
        document.getElementById('favicon').href = branding.favicon;
    }
    if (branding.background_color !== '') {
        document.body.style.background = branding.background_color;
    }
    if (branding.header_color !== '') {
        document.getElementsByTagName('header')[0].style.backgroundColor = branding.header_color;
    }
}
