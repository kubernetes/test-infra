'use strict';

window.onload = () => {
    const queries = urlQueries(window.location.search);
    const queryMapping = new Map(Object.entries({
        "prow_job_id": function (prowJob) {
            return prowJob.metadata.name;
        },
        "job": function (prowJob) {
            return prowJob.spec.job;
        },
        "build": function (prowJob) {
            if (typeof prowJob.status.build !== "undefined") {
                return prowJob.status.build;
            } else {
                return null;
            }
        },
        "org": function (prowJob) {
            if (typeof prowJob.spec.refs !== "undefined") {
                return prowJob.spec.refs.org;
            } else {
                return null;
            }
        },
        "repo": function (prowJob) {
            if (typeof prowJob.spec.refs !== "undefined") {
                return prowJob.spec.refs.repo;
            } else {
                return null;
            }
        },
        "author": function (prowJob) {
            if (typeof prowJob.spec.refs !== "undefined" && typeof prowJob.spec.refs.pulls !== "undefined") {
                return prowJob.spec.refs.pulls[0].author;
            } else {
                return null;
            }
        },
        "agent": function (prowJob) {
            return prowJob.spec.agent;
        },
        "type": function (prowJob) {
            return prowJob.spec.type;
        },
    }));
    const prowJobs = allJobs.items.filter(job => {
        let matches = true;
        for (let [key, valueExtractor] of queryMapping) {
            if (queries.has(key)) {
                matches = matches && (valueExtractor(job) === queries.get(key));
            }
        }
        return matches;
    });
    // TODO: figure out what to do with many results,
    // right now that DOS the client trying to load logs
    redraw(prowJobs.slice(0, 10));
};

/**
 * Return a key-value mapping of all queries encoded in the URI.
 *
 * @param {string} raw
 * @returns {Map}
 */
function urlQueries(raw) {
    const queries = raw.slice(raw.indexOf(`?`) + 1).split(`&`);
    return queries.reduce((accumulated, query) => {
        const [key, value] = query.split(`=`);
        accumulated.set(key, decodeURIComponent(value));
        return accumulated
    }, new Map());
}

/**
 * Redraw the page
 *
 * @param {Array} prowJobs
 */
function redraw(prowJobs) {
    const mainContainer = document.querySelector("#main-container");
    while (mainContainer.firstChild) {
        mainContainer.removeChild(mainContainer.firstChild);
    }
    if (prowJobs.length === 0) {
        mainContainer.appendChild(document.createTextNode("No jobs matched."))
    } else {
        for (const prowJob of prowJobs) {
            mainContainer.appendChild(jobCard(prowJob))
        }
    }
}

function jobCard(prowJob) {
    const card = document.createElement("DIV");
    card.id = "job-card";
    card.classList.add("job-card", "mdl-card", "mdl-shadow--2dp");
    card.appendChild(jobCardTitle(prowJob));
    card.appendChild(jobCardBody(prowJob));
    return card;
}

function descriptionForState(state) {
    switch (state) {
        case "success":
            return "Job Succeeded";
        case "failure":
            return "Job Failed";
        case "pending":
            return "Job Pending";
        case "triggered":
            return "Job Triggered";
        case "aborted":
            return "Job Aborted";
        case "error":
            return "Internal Error";
        default:
            return "Unknown Job State";
    }
}

function iconForState(state) {
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
}

function jobCardTitle(prowJob) {
    const cardTitle = document.createElement("DIV");
    cardTitle.classList.add("mdl-card__title", "title-state-" + prowJob.status.state);
    cardTitle.id = `${prowJob.spec.job}-${prowJob.status.build_id}-title`;

    const icon = document.createElement("I");
    icon.classList.add("title-icon", "material-icons");
    icon.textContent = iconForState(prowJob.status.state);
    const title = document.createElement("H4");
    title.classList.add("mdl-card__title-text");
    const titleContainer = document.createElement("SPAN");
    const jobName = document.createElement("A");
    jobName.textContent = prowJob.spec.job;
    jobName.href = "job?job=" + prowJob.spec.job;
    const buildId = document.createElement("A");
    buildId.textContent = prowJob.status.build_id;
    buildId.href = "job?prow_job_id=" + prowJob.metadata.name;

    const tooltip = document.createElement("DIV");
    tooltip.textContent = descriptionForState(prowJob.status.state);
    tooltip.setAttribute("data-mdl-for", cardTitle.id);
    tooltip.classList.add("mdl-tooltip", "mdl-tooltip--large");

    cardTitle.appendChild(tooltip);
    title.appendChild(icon);
    titleContainer.appendChild(jobName);
    titleContainer.appendChild(document.createTextNode(" #"));
    titleContainer.appendChild(buildId);
    title.appendChild(titleContainer);
    cardTitle.appendChild(title);
    return cardTitle;
}

function jobCardBody(prowJob) {
    const cardBody = document.createElement("DIV");
    cardBody.classList.add("mdl-card__supporting-text");
    cardBody.appendChild(metadataTable(prowJob));
    if (prowJob.spec.agent === "kubernetes") {
        cardBody.appendChild(podSpecTable(prowJob.spec.pod_spec));
    }
    cardBody.appendChild(jobLogTable(prowJob.spec.job, prowJob.status.build_id));
    return cardBody;
}

function metadataTable(prowJob) {
    const table = document.createElement("TABLE");
    table.classList.add("metadata-table");
    const body = document.createElement("TBODY");
    const row = document.createElement("TR");
    row.appendChild(specColumn(prowJob));
    row.appendChild(timeColumn(prowJob));
    body.appendChild(row);
    table.appendChild(body);
    return table;
}

function specColumn(prowJob) {
    const specColumn = document.createElement("TD");
    const specList = document.createElement("UL");
    specList.classList.add("mdl-list");

    if (prowJob.spec.type !== "periodic") {
        const baseRef = document.createElement("LI");
        baseRef.classList.add("mdl-list__item");
        const baseRefContent = document.createElement("PRE");
        baseRefContent.classList.add("mdl-list__item-primary-content");
        const repoLink = document.createElement("A");
        repoLink.href = `https://www.github.com/${prowJob.spec.refs.org}/${prowJob.spec.refs.repo}`;
        repoLink.textContent = `${prowJob.spec.refs.org}/${prowJob.spec.refs.repo}`;
        const baseRefLink = document.createElement("A");
        baseRefLink.href = `https://www.github.com/${prowJob.spec.refs.org}/${prowJob.spec.refs.repo}/tree/${prowJob.spec.refs.base_ref}`;
        baseRefLink.textContent = " " + prowJob.spec.refs.base_ref;
        const baseShaLink = document.createElement("A");
        baseShaLink.href = `https://www.github.com/${prowJob.spec.refs.org}/${prowJob.spec.refs.repo}/commits/${prowJob.spec.refs.base_sha}`;
        baseShaLink.textContent = prowJob.spec.refs.base_sha.substring(0, 7);
        const baseIcon = document.createElement("I");
        baseIcon.classList.add("material-icons", "mdl-list__item-icon");
        baseIcon.textContent = "code";

        baseRefContent.appendChild(repoLink);
        baseRefContent.appendChild(baseRefLink);
        baseRefContent.appendChild(document.createTextNode(" branch at "));
        baseRefContent.appendChild(baseShaLink);
        baseRef.appendChild(baseIcon);
        baseRef.appendChild(baseRefContent);
        specList.appendChild(baseRef);

        if (prowJob.spec.type === "presubmit" || prowJob.spec.type === "batch") {
            for (const pr of prowJob.spec.refs.pulls) {
                const pull = document.createElement("LI");
                pull.classList.add("mdl-list__item");
                const pullContent = document.createElement("PRE");
                pullContent.classList.add("mdl-list__item-primary-content");
                const pullLink = document.createElement("A");
                pullLink.href = `https://www.github.com/${prowJob.spec.refs.org}/${prowJob.spec.refs.repo}/pull/${pr.number}`;
                pullLink.textContent = pr.number;
                const pullAuthorLink = document.createElement("A");
                pullAuthorLink.href = "https://www.github.com/" + pr.author;
                pullAuthorLink.textContent = pr.author;
                const pullShaLink = document.createElement("A");
                pullShaLink.href = `https://www.github.com/${prowJob.spec.refs.org}/${prowJob.spec.refs.repo}/commits/${pr.sha}`;
                pullShaLink.textContent = pr.sha.substring(0, 7);
                const baseIcon = document.createElement("I");
                baseIcon.classList.add("material-icons", "mdl-list__item-icon");
                baseIcon.textContent = "code";

                pullContent.appendChild(document.createTextNode("pull #"));
                pullContent.appendChild(pullLink);
                pullContent.appendChild(document.createTextNode(" by "));
                pullContent.appendChild(pullAuthorLink);
                pullContent.appendChild(document.createTextNode(" at "));
                pullContent.appendChild(pullShaLink);
                pull.appendChild(baseIcon);
                pull.appendChild(pullContent);
                specList.appendChild(pull);
            }
        }
    } else {
        const info = document.createElement("LI");
        info.classList.add("mdl-list__item");
        const infoContent = document.createElement("SPAN");
        infoContent.classList.add("mdl-list__item-primary-content");
        infoContent.textContent = "No repositories configured to clone.";
        info.appendChild(infoContent);
        specList.appendChild(info);
    }
    specColumn.appendChild(specList);
    return specColumn;
}

function timeColumn(prowJob) {
    const timeColumn = document.createElement("TD");
    timeColumn.classList.add("time-column");
    const timeList = document.createElement("UL");
    timeList.classList.add("mdl-list");

    const startedAt = document.createElement("LI");
    startedAt.classList.add("mdl-list__item");
    const startedAtContent = document.createElement("SPAN");
    startedAtContent.classList.add("mdl-list__item-primary-content");
    startedAtContent.id = `${prowJob.metadata.name}-started-at`;
    if (prowJob.status.startTime !== "") {
        const startTimeAsMoment = moment.utc(prowJob.status.startTime);
        if (startTimeAsMoment.isBefore(moment().startOf('day'))) {
            startedAtContent.textContent = "started on " + startTimeAsMoment.format('MMM DD [at] LTS');
        } else {
            startedAtContent.textContent = "started at " + startTimeAsMoment.format('LTS');
        }
        const startedAtTooltip = document.createElement("DIV");
        startedAtTooltip.textContent = startTimeAsMoment.format('MMM DD YYYY, LTS');
        startedAtTooltip.setAttribute("data-mdl-for", startedAtContent.id);
        startedAtTooltip.classList.add("mdl-tooltip", "mdl-tooltip--large");
        startedAt.appendChild(startedAtTooltip);
    } else {
        startedAtContent.textContent = "not yet started";
    }
    const startedAtIcon = document.createElement("I");
    startedAtIcon.classList.add("material-icons", "mdl-list__item-icon");
    startedAtIcon.textContent = "schedule";

    startedAt.appendChild(startedAtIcon);
    startedAt.appendChild(startedAtContent);
    timeList.appendChild(startedAt);

    const duration = document.createElement("LI");
    duration.classList.add("mdl-list__item");
    const durationContent = document.createElement("SPAN");
    durationContent.classList.add("mdl-list__item-primary-content");
    if (prowJob.status.completionTime !== "" && prowJob.status.startTime !== "") {
        const durationAsMoment = moment.utc(moment.utc(prowJob.status.completionTime).diff(moment.utc(prowJob.status.startTime)));
        let verb = "";
        switch (prowJob.status.state) {
            case "success":
                verb = "succeeded";
                break;
            case "failure":
                verb = "failed";
                break;
            case "aborted":
                verb = "aborted";
                break;
        }
        durationContent.textContent = verb + " after " + durationAsMoment.format('HH[h] mm[m] ss[s]');
    } else {
        let verb = "";
        switch (prowJob.status.state) {
            case "pending":
                verb = "pending";
                break;
            case "triggered":
                verb = "running";
                break;
        }
        durationContent.textContent = "still " + verb;
    }
    const durationIcon = document.createElement("I");
    durationIcon.classList.add("material-icons", "mdl-list__item-icon");
    durationIcon.textContent = "history";

    duration.appendChild(durationIcon);
    duration.appendChild(durationContent);
    timeList.appendChild(duration);
    timeColumn.appendChild(timeList);
    return timeColumn;
}

function podSpecTable(podSpecRaw) {
    const podSpecTitle = document.createElement("H5");
    podSpecTitle.textContent = "View Kubernetes ";
    const podSpecText = document.createElement("CODE");
    podSpecText.textContent = "PodSpec";
    podSpecTitle.appendChild(podSpecText);

    const podSpecTableName = "podSpec";
    const podSpecYAML = codeTable(JSON.stringify(podSpecRaw, null, 2), podSpecTableName);

    return collapsableTable(podSpecTitle, podSpecYAML, podSpecTableName);
}

function collapsableTable(title, content, name) {
    const container = document.createElement("DIV");
    const arrow = document.createElement("BUTTON");
    arrow.classList.add("mdl-button", "mdl-js-button", "mdl-button--icon");
    const arrowIcon = document.createElement("I");
    arrowIcon.classList.add("icon-button", "material-icons");
    arrowIcon.textContent = "expand_less";
    arrow.appendChild(arrowIcon);
    if (!window.location.hash.startsWith("#" + name)) {
        content.classList.add("hidden");
        arrowIcon.textContent = "expand_more";
    }

    title.addEventListener("click", () => {
        content.classList.toggle("hidden");
        if (arrowIcon.textContent === "expand_more") {
            arrowIcon.textContent = "expand_less";
        } else {
            arrowIcon.textContent = "expand_more";
        }
    });

    title.appendChild(arrow);
    container.appendChild(title);
    container.appendChild(content);
    return container;
}

function codeTable(code, name) {
    const container = document.createElement("DIV");
    container.classList.add("code-table-container");
    const table = document.createElement("TABLE");
    table.classList.add("code-table");
    const body = document.createElement("TBODY");
    let i = 0;
    for (const line of code.split("\n")) {
        const row = document.createElement("TR");
        row.classList.add("code-row");
        const number = document.createElement("TD");
        number.classList.add("line-number");
        number.id = name + "-L" + i;
        const numberLink = document.createElement("A");
        numberLink.textContent = i;
        const linkFragment = "#" + number.id;
        numberLink.href = linkFragment; // TODO: redraw table to highlight new line
        const content = document.createElement("TD");
        content.textContent = line;
        content.classList.add("line-code");
        if (window.location.hash === linkFragment) {
            content.classList.add("highlight");
        }

        number.appendChild(numberLink);
        row.appendChild(number);
        row.appendChild(content);
        body.appendChild(row);
        i++;
    }

    table.appendChild(body);
    container.appendChild(table);
    return container;
}

function jobLogTable(job, buildId) {
    const jobLogTitle = document.createElement("H5");
    jobLogTitle.textContent = "View Log";

    const logTableName = "log";
    const logContent = document.createElement("DIV");
    logContent.id = job + "-" + buildId + "-logs";
    const logContentText = document.createTextNode("Loading job logs...");
    logContent.appendChild(logContentText);

    const table = collapsableTable(jobLogTitle, logContent, logTableName);

    const request = new XMLHttpRequest();
    const url = "/log?job=" + job + "&id=" + buildId;
    request.onreadystatechange = () => {
        if (request.readyState !== 4) {
            return
        }
        const container = document.querySelector("#" + logContent.id);
        while (container.firstChild) {
            container.removeChild(container.firstChild);
        }
        let updatedLogContent;
        if (request.status === 200) {
            updatedLogContent = codeTable(request.responseText, logTableName);
        } else if (request.status === 404) {
            // we may have deleted the pod, but
            // can grab logs from GCS instead
            updatedLogContent = document.createElement("DIV");
            updatedLogContent.textContent = "TODO: load from GCS";
        } else {
            updatedLogContent = document.createElement("DIV");
            updatedLogContent.textContent = "Failed to load job logs.";
        }
        container.appendChild(updatedLogContent);
    };
    request.withCredentials = true;
    request.open("GET", url, true);
    request.setRequestHeader("Content-type", "application/x-www-form-urlencoded");
    request.send();

    return table;
}

document.addEventListener("DOMContentLoaded", function () {
    configure();
});

function configure() {
    if (typeof branding === "undefined") {
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