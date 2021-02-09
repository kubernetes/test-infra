import dialogPolyfill from "dialog-polyfill";

import {Context} from '../api/github';
import {Label, PullRequest, UserData} from '../api/pr';
import {ProwJob, ProwJobList, ProwJobState} from '../api/prow';
import {Blocker, TideData, TidePool, TideQuery as ITideQuery} from '../api/tide';
import {getCookieByName, tidehistory} from '../common/common';
import {relativeURL} from "../common/urls";

declare const tideData: TideData;
declare const allBuilds: ProwJobList;
declare const csrfToken: string;

type UnifiedState = ProwJobState | "expected";

interface UnifiedContext {
  context: string;
  description: string;
  state: UnifiedState;
  discrepancy: string | null;
  url?: string;
}

interface ProcessedLabel {
  name: string;
  own: boolean;
}

interface ProcessedQuery {
  score: number;
  labels: ProcessedLabel[];
  missingLabels: ProcessedLabel[];
  excludedBranches?: string[];
  includedBranches?: string[];
  author?: string;
  milestone?: string;
}

/**
 * A Tide Query helper class that checks whether a pr is covered by the query.
 */
class TideQuery {
    public orgs?: string[];
    public repos?: string[];
    public excludedRepos?: string[];
    public labels?: string[];
    public missingLabels?: string[];
    public excludedBranches?: string[];
    public includedBranches?: string[];
    public author?: string;
    public milestone?: string;

    constructor(query: ITideQuery) {
        this.orgs = query.orgs;
        this.repos = query.repos;
        this.excludedRepos = query.excludedRepos;
        this.labels = query.labels;
        this.missingLabels = query.missingLabels;
        this.excludedBranches = query.excludedBranches;
        this.includedBranches = query.includedBranches;
        this.author = query.author;
        this.milestone = query.milestone;
    }

    /**
     * Returns true if the pr is covered by the query.
     */
    public matchPr(pr: PullRequest): boolean {
        const isMatched =
            (this.repos && this.repos.indexOf(pr.Repository.NameWithOwner) !== -1) ||
            ((this.orgs && this.orgs.indexOf(pr.Repository.Owner.Login) !== -1) &&
            (!this.excludedRepos || this.excludedRepos.indexOf(pr.Repository.NameWithOwner) === -1));

        if (!isMatched) {
            return false;
        }

        if (pr.BaseRef) {
            if (this.excludedBranches &&
                this.excludedBranches.indexOf(pr.BaseRef.Name) !== -1) {
                return false;
            }
            if (this.includedBranches &&
                this.includedBranches.indexOf(pr.BaseRef.Name) === -1) {
                return false;
            }
        }

        return true;
    }
}

/**
 * Submit the query by redirecting the page with the query and let window.onload
 * sends the request.
 * @param input query input element
 */
function submitQuery(input: HTMLInputElement | HTMLTextAreaElement) {
    const query = getPRQuery(input.value);
    input.value = query;
    window.location.search = '?query=' + encodeURIComponent(query);
}

/**
 * Creates a XMLHTTP request to /pr-data.js.
 * @param {function} fulfillFn
 * @param {function} errorHandler
 */
function createXMLHTTPRequest(fulfillFn: (request: XMLHttpRequest) => any, errorHandler: () => any): XMLHttpRequest {
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
    request.setRequestHeader("X-CSRF-Token", csrfToken);

    return request;
}

/**
 * Makes sure the search query is looking for pull requests by dropping all
 * is:issue and type:pr tokens and adds is:pr if does not exist.
 */
function getPRQuery(q: string): string {
    const tokens = q.replace(/\+/g, " ").split(" ");
    // Firstly, drop all pr/issue search tokens
    let result = tokens.filter((tkn) => {
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
 */
function redraw(prData: UserData): void {
    const mainContainer = document.querySelector("#pr-container")!;
    while (mainContainer.firstChild) {
        mainContainer.removeChild(mainContainer.firstChild!);
    }
    if (prData && prData.Login) {
        loadPrStatus(prData);
    } else {
        forceGitHubLogin();
    }
}

/**
 * Enables/disables the progress bar.
 */
function loadProgress(isStarted: boolean): void {
    const pg = document.querySelector("#loading-progress")!;
    if (isStarted) {
        pg.classList.remove("hidden");
    } else {
        pg.classList.add("hidden");
    }
}

/**
 * Handles the URL query on load event.
 */
function onLoadQuery(): string {
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
 * Creates an alert for merge blocking issues on tide.
 */
function createMergeBlockingIssueAlert(tidePool: TidePool, blockers: Blocker[]): HTMLElement {
    const alert = document.createElement("div");
    alert.classList.add("alert");
    alert.textContent = `Currently Prow is not merging any PRs to ${tidePool.Org}/${tidePool.Repo} on branch ${tidePool.Branch}. Refer to `;

    for (let j = 0; j < blockers.length; j++) {
        const issue = blockers[j];
        const link = document.createElement("a");
        link.href = issue.URL;
        link.innerText = "#" + issue.Number;
        if (j + 1 < blockers.length) {
            link.innerText = link.innerText + ", ";
        }
        alert.appendChild(link);
    }
    const closeButton = document.createElement("span");
    closeButton.textContent = "×";
    closeButton.classList.add("closebutton");
    closeButton.addEventListener("click", () => {
        alert.classList.add("hidden");
    });
    alert.appendChild(closeButton);
    return alert;
}

/**
 * Displays any status alerts, e.g: tide pool blocking issues.
 */
function showAlerts(): void {
    const alertContainer = document.querySelector("#alert-container")!;
    const tidePools = tideData.Pools ? tideData.Pools : [];
    for (const pool of tidePools) {
        const blockers = pool.Blockers ? pool.Blockers : [];
        if (blockers.length > 0) {
            alertContainer.appendChild(createMergeBlockingIssueAlert(pool, blockers));
        }
    }
}

window.onload = () => {
    const dialogs = document.querySelectorAll("dialog") as NodeListOf<HTMLDialogElement>;
    dialogs.forEach((dialog) => {
        dialogPolyfill.registerDialog(dialog);
        dialog.querySelector('.close')!.addEventListener('click', () => {
            dialog.close();
        });
    });
    // Check URL, if the search is empty, adds search query by default format
    // ?is:pr state:open query="author:<user_login>"
    if (window.location.search === "") {
        const login = getCookieByName("github_login");
        const searchQuery = "is:pr state:open author:" + login;
        window.location.search = "?query=" + encodeURIComponent(searchQuery);
    }
    const request = createXMLHTTPRequest((r) => {
        const prData = JSON.parse(r.responseText);
        redraw(prData);
        loadProgress(false);
    }, () => {
        loadProgress(false);
        const mainContainer = document.querySelector("#pr-container")!;
        mainContainer.appendChild(createMessage("Something wrongs! We could not fulfill your request"));
    });
    showAlerts();
    loadProgress(true);
    request.send("query=" + onLoadQuery());
};

function createSearchCard(): HTMLElement {
    const searchCard = document.createElement("div");
    searchCard.id = "search-card";
    searchCard.classList.add("pr-card", "mdl-card");

    // Input box
    const input = document.createElement("textarea");
    input.id = "search-input";
    input.value = decodeURIComponent(window.location.search.slice("query=".length + 1));
    input.rows = 1;
    input.spellcheck = false;
    input.addEventListener("keydown", (event) => {
        if (event.keyCode === 13) {
            event.preventDefault();
            submitQuery(input);
        } else {
            const el = event.target as HTMLTextAreaElement;
            el.style.height = "auto";
            el.style.height = el.scrollHeight + "px";
        }
    });
    input.addEventListener("focus", (event) => {
        const el = event.target as HTMLTextAreaElement;
        el.style.height = "auto";
        el.style.height = el.scrollHeight + "px";
    });
    // Refresh button
    const refBtn = createIcon("refresh", "Reload the query", ["search-button"], true);
    refBtn.addEventListener("click", () => {
        submitQuery(input);
    }, true);
    const userBtn = createIcon("person", "Show my open pull requests", ["search-button"], true);
    userBtn.addEventListener("click", () => {
        const login = getCookieByName("github_login");
        const searchQuery = "is:pr state:open author:" + login;
        window.location.search = "?query=" + encodeURIComponent(searchQuery);
    });

    const actionCtn = document.createElement("div");
    actionCtn.id = "search-action";
    actionCtn.appendChild(userBtn);
    actionCtn.appendChild(refBtn);
    actionCtn.appendChild(tidehistory.authorIcon(getCookieByName("github_login")));

    const inputContainer = document.createElement("div");
    inputContainer.id = "search-input-ctn";
    inputContainer.appendChild(input);
    inputContainer.appendChild(actionCtn);

    const title = document.createElement("h6");
    title.textContent = "GitHub search query";
    const infoBtn = createIcon("info", "More information about the search query", ["search-info"], true);
    const titleCtn = document.createElement("div");
    titleCtn.appendChild(title);
    titleCtn.appendChild(infoBtn);
    titleCtn.classList.add("search-title");

    const searchDialog = document.querySelector("#search-dialog") as HTMLDialogElement;
    infoBtn.addEventListener("click", () => {
        searchDialog.showModal();
    });

    searchCard.appendChild(titleCtn);
    searchCard.appendChild(inputContainer);
    return searchCard;
}

/**
 * GetFullPRContexts gathers build jobs and pr contexts. It firstly takes
 * all pr contexts and only replaces contexts that have existing Prow Jobs. Tide
 * context will be omitted from the list.
 */
function getFullPRContext(builds: ProwJob[], contexts: Context[]): UnifiedContext[] {
    const contextMap: Map<string, UnifiedContext> = new Map();
    if (contexts) {
        for (const context of contexts) {
            if (context.Context === "tide") {
                continue;
            }
            contextMap.set(context.Context, {
                context: context.Context,
                description: context.Description,
                discrepancy: null,
                state: context.State.toLowerCase() as UnifiedState,
            });
        }
    }

    for (const build of builds) {
        const {
            spec: {
                context = "",
            },
            status: {
                url = "", description = "", state = "",
            },
        } = build;

        let discrepancy = null;
        // If GitHub context exits, check if mismatch or not.
        if (contextMap.has(context)) {
            const githubContext = contextMap.get(context)!;
            // TODO (qhuynh96): ProwJob's states and GitHub contexts states
            // are not equivalent in some states.
            if (githubContext.state !== state) {
                discrepancy = "GitHub context and Prow Job states mismatch";
            }
        }
        contextMap.set(context, {
            context,
            description,
            discrepancy,
            state,
            url,
        });
    }

    return Array.from(contextMap.values());
}

/**
 * Loads Pr Status
 */
function loadPrStatus(prData: UserData): void {
    const tideQueries: TideQuery[] = [];
    if (tideData.TideQueries) {
        for (const query of tideData.TideQueries) {
            tideQueries.push(new TideQuery(query));
        }
    }

    const container = document.querySelector("#pr-container")!;
    container.appendChild(createSearchCard());
    if (!prData.PullRequestsWithContexts || prData.PullRequestsWithContexts.length === 0) {
        const msg = createMessage("No open PRs found", "");
        container.appendChild(msg);
        return;
    }
    for (const prWithContext of prData.PullRequestsWithContexts) {
        // There might be multiple runs of jobs for a build.
        // allBuilds is sorted with the most recent builds first, so
        // we only need to keep the first build for each job name.
        const pr = prWithContext.PullRequest;
        const seenJobs: {[key: string]: boolean} = {};
        const builds: ProwJob[] = [];
        for (const build of allBuilds.items) {
            const {
                spec: {
                    type = "",
                    job = "",
                    refs: {repo = "", pulls = [], base_ref = ""} = {},
                },
            } = build;

            if (type === 'presubmit' &&
                repo === pr.Repository.NameWithOwner &&
                base_ref === pr.BaseRef.Name &&
                pulls.length &&
                pulls[0].number === pr.Number &&
                pulls[0].sha === pr.HeadRefOID) {
                if (!seenJobs[job]) {  // First (latest) build for job.
                    seenJobs[job] = true;
                    builds.push(build);
                }
            }
        }
        const githubContexts = prWithContext.Contexts;
        const contexts = getFullPRContext(builds, githubContexts);
        const validQueries: TideQuery[] = [];
        for (const query of tideQueries) {
            if (query.matchPr(pr)) {
                validQueries.push(query);
            }
        }
        container.appendChild(createPRCard(pr, contexts, closestMatchingQueries(pr, validQueries), tideData.Pools));
    }
}

/**
 * Creates Pool labels.
 */
function createTidePoolLabel(pr: PullRequest, tidePool?: TidePool): HTMLElement | null {
    if (!tidePool) {
        return null;
    }
    const label = document.createElement("span");
    const blockers = tidePool.Blockers ? tidePool.Blockers : [];
    if (blockers.length > 0) {
        label.textContent = "Pool is temporarily blocked";
        label.classList.add("title-label", "mdl-shadow--2dp", "pending");
        return label;
    }
    const poolTypes = [tidePool.Target, tidePool.BatchPending,
        tidePool.SuccessPRs, tidePool.PendingPRs, tidePool.MissingPRs];
    const inPoolId = poolTypes.findIndex((poolType) => {
        if (!poolType) {
            return false;
        }
        const index = poolType.findIndex((prInPool) => {
            return prInPool.Number === pr.Number;
        });
        return index !== -1;
    });
    if (inPoolId === -1) {
        return null;
    }
    const labelTitle = ["Merging", "In Batch & Test Pending",
        "Test Passing & Merge Pending", "Test Pending",
        "Queued for retest"];
    const labelStyle = ["merging", "batching", "passing", "pending", "pending"];
    label.textContent = "In Pool - " + labelTitle[inPoolId];
    label.classList.add("title-label", "mdl-shadow--2dp", labelStyle[inPoolId]);

    return label;
}

/**
 * Creates a label for the title. It will prioritise the merge status over the
 * job status. Saying that, if the pr has jobs failed and does not meet merge
 * requirements, it will show that the PR needs to resolve labels.
 */
function createTitleLabel(isMerge: boolean, jobStatus: VagueState, noQuery: boolean,
                          labelConflict: boolean, mergeConflict: boolean,
                          branchConflict: boolean, authorConflict: boolean, milestoneConflict: boolean): HTMLElement {
    const label = document.createElement("span");
    label.classList.add("title-label");

    if (noQuery) {
        label.textContent = "Unknown Merge Requirements";
        label.classList.add("unknown");
    } else if (isMerge) {
        label.textContent = "Merged";
        label.classList.add("merging");
    } else if (authorConflict) {
        label.textContent = "Blocked from merging by current author";
        label.classList.add("pending");
    } else if (branchConflict) {
        label.textContent = "Blocked from merging into target branch";
        label.classList.add("pending");
    } else if (milestoneConflict) {
        label.textContent = "Blocked from merging by current milestone";
        label.classList.add("pending");
    } else if (mergeConflict) {
        label.textContent = "Needs to resolve merge conflicts";
        label.classList.add("pending");
    } else if (labelConflict) {
        label.textContent = "Needs to resolve labels";
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
 */
function createPRCardTitle(pr: PullRequest, tidePools: TidePool[], jobStatus: VagueState,
                           noQuery: boolean, labelConflict: boolean,
                           mergeConflict: boolean, branchConflict: boolean,
                           authorConflict: boolean, milestoneConflict: boolean): HTMLElement {
    const prTitle = document.createElement("div");
    prTitle.classList.add("mdl-card__title");

    const title = document.createElement("h4");
    title.textContent = "#" + pr.Number;
    title.classList.add("mdl-card__title-text");

    const subtitle = document.createElement("h5");
    subtitle.textContent = `${pr.Repository.NameWithOwner}:${pr.BaseRef.Name}`;
    subtitle.classList.add("mdl-card__subtitle-text");

    const link = document.createElement("a");
    link.href = `/github-link?dest=${pr.Repository.NameWithOwner}/pull/${pr.Number}`;
    link.appendChild(title);

    const prTitleText = document.createElement("div");
    prTitleText.appendChild(link);
    prTitleText.appendChild(subtitle);
    prTitleText.appendChild(document.createTextNode("\u00A0"));
    prTitleText.appendChild(tidehistory.poolIcon(pr.Repository.Owner.Login, pr.Repository.Name, pr.BaseRef.Name));
    prTitleText.classList.add("pr-title-text");
    prTitle.appendChild(prTitleText);

    const pool = tidePools.filter((p) => {
        const repo = `${p.Org}/${p.Repo}`;
        return pr.Repository.NameWithOwner === repo && pr.BaseRef.Name === p.Branch;
    });
    let tidePoolLabel = createTidePoolLabel(pr, pool[0]);
    if (!tidePoolLabel) {
        tidePoolLabel = createTitleLabel(pr.Merged, jobStatus, noQuery, labelConflict, mergeConflict, branchConflict, authorConflict, milestoneConflict);
    }
    prTitle.appendChild(tidePoolLabel);

    return prTitle;
}

/**
 * Creates a list of contexts.
 */
function createContextList(contexts: UnifiedContext[], itemStyle: string[] = []): HTMLElement {
    const container = document.createElement("ul");
    container.classList.add("mdl-list", "job-list");
    const getStateIcon = (state: string): string => {
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
    const getItemContainer = (context: UnifiedContext): HTMLElement => {
        if (context.url) {
            const item = document.createElement("a");
            item.href = context.url;
            return item;
        } else {
            return document.createElement("div");
        }
    };
    contexts.forEach((context) => {
        const elCon = document.createElement("li");
        elCon.classList.add("mdl-list__item", "job-list-item", ...itemStyle);
        const item = getItemContainer(context);
        item.classList.add("mdl-list__item-primary-content");
        item.appendChild(createIcon(
            getStateIcon(context.state),
            "",
            ["state", context.state, "mdl-list__item-icon"]));
        item.appendChild(document.createTextNode(context.context));
        if (context.discrepancy) {
            item.appendChild(createIcon(
                "warning",
                context.discrepancy,
                ["state", "context-warning", "mdl-list__item-icon"]));
        }
        elCon.appendChild(item);
        if (context.description) {
            const itemDesc = document.createElement("span");
            itemDesc.textContent = context.description;
            itemDesc.style.color = "grey";
            itemDesc.style.fontSize = "14px";
            elCon.appendChild(itemDesc);
        }
        container.appendChild(elCon);
    });
    return container;
}

/**
 * Creates Job status.
 */
function createJobStatus(builds: UnifiedContext[]): HTMLElement {
    const statusContainer = document.createElement("div");
    statusContainer.classList.add("status-container");
    const status = document.createElement("div");
    const failedJobs = builds.filter((build) => {
        return build.state === "failure";
    });
    // Job status indicator
    const state = jobVagueState(builds);
    let statusText = "";
    let stateIcon = "";
    switch (state) {
        case "succeeded":
            statusText = "All tests passed";
            stateIcon = "check_circle";
            break;
        case "failed":
            statusText = `${failedJobs.length} test${(failedJobs.length === 1 ? "" : "s")} failed`;
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
    status.classList.add("status", "expandable");
    statusContainer.appendChild(status);
    // Job list
    let failedJobsList: HTMLElement | undefined;
    if (failedJobs.length > 0) {
        failedJobsList = createContextList(failedJobs);
        statusContainer.appendChild(failedJobsList);
    }
    const jobList = createContextList(builds);
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
 * escapeLabel escaped label name that returns a valid name used for css
 * selector.
 */
function escapeLabel(label: string): string {
    if (label === "") { return ""; }
    const toUnicode = (index: number): string => {
      const h = label.charCodeAt(index).toString(16).split('');
      while (h.length < 6) { h.splice(0, 0, '0'); }

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

    return result;
}

/**
 * Creates a HTML element for the label given its name
 */
function createLabelEl(label: string): HTMLElement {
    const el = document.createElement("span");
    const escapedName = escapeLabel(label);
    el.classList.add("merge-table-label", "mdl-shadow--2dp", "label", escapedName);
    el.textContent = label;

    return el;
}

/**
 * Creates a merge requirement cell.
 */
function createMergeLabelCell(labels: ProcessedLabel[], notMissingLabel = false): HTMLElement {
    const cell = document.createElement("td");
    labels.forEach((label) => {
        const labelEl = createLabelEl(label.name);
        const toDisplay = label.own !== notMissingLabel;
        if (toDisplay) {
            cell.appendChild(labelEl);
        }
    });

    return cell;
}

/**
 * Appends labels to a container
 */
function appendLabelsToContainer(container: HTMLElement, labels: string[]): void {
    while (container.firstChild) {
        container.removeChild(container.firstChild);
    }
    labels.forEach((label) => {
        const labelEl = createLabelEl(label);
        container.appendChild(labelEl);
    });
}

/**
 * Fills query details. The details will be either the author, milestone or
 * included/excluded branches.
 * @param selector
 * @param data
 */
function fillDetail(selector: string, data?: string[] | string): void {
    const section = document.querySelector(selector)!;
    if (!data || (Array.isArray(data) && data.length === 0)) {
        section.classList.add("hidden");
        return;
    }

    section.classList.remove("hidden");
    const container = section.querySelector(".detail-data")!;
    container.textContent = "";
    while (container.firstChild) {
        container.removeChild(container.firstChild);
    }

    if (Array.isArray(data)) {
        for (const branch of data) {
            const str = document.createElement("SPAN");
            str.classList.add("detail-branch");
            str.appendChild(document.createTextNode(branch));
            container.appendChild(str);
        }
    } else if (typeof data === 'string') {
        container.appendChild(document.createTextNode(data));
    }
}

/**
 * Creates query details btn
 * @param query
 * @returns {HTMLElement}
 */
function createQueryDetailsBtn(query: ProcessedQuery): HTMLTableDataCellElement {
    const mergeIcon = document.createElement("td");
    mergeIcon.classList.add("merge-table-icon");

    const iconButton = createIcon("information", "Clicks to see query details", [], true);
    const dialog = document.querySelector("#query-dialog")! as HTMLDialogElement;

    // Query labels
    const allRequired = document.getElementById("query-all-required")!;
    const allForbidden = document.getElementById("query-all-forbidden")!;
    iconButton.addEventListener("click", () => {
        fillDetail("#query-detail-author", query.author);
        fillDetail("#query-detail-milestone", query.milestone);
        fillDetail("#query-detail-exclude", query.excludedBranches);
        fillDetail("#query-detail-include", query.includedBranches);
        appendLabelsToContainer(allRequired, query.labels.map((label) => {
            return label.name;
        }));
        appendLabelsToContainer(allForbidden, query.missingLabels.map((label) => {
            return label.name;
        }));
        dialog.showModal();
    });
    mergeIcon.appendChild(iconButton);

    return mergeIcon;
}

/**
 * Creates merge requirement table for queries.
 */
function createQueriesTable(prLabels: Array<{Label: Label}>, queries: ProcessedQuery[]): HTMLTableElement {
    const table = document.createElement("table");
    table.classList.add("merge-table");
    const thead = document.createElement("thead");
    const allLabelHeaderRow = document.createElement("tr");
    const allLabelHeaderCell = document.createElement("td");
    // Creates all pr labels header.
    allLabelHeaderCell.textContent = "PR's Labels";
    allLabelHeaderCell.colSpan = 3;
    allLabelHeaderRow.appendChild(allLabelHeaderCell);
    thead.appendChild(allLabelHeaderRow);

    const allLabelRow = document.createElement("tr");
    const allLabelCell = document.createElement("td");
    allLabelCell.colSpan = 3;
    appendLabelsToContainer(allLabelCell, prLabels.map((label) => {
        return label.Label.Name;
    }));
    allLabelRow.appendChild(allLabelCell);
    thead.appendChild(allLabelRow);

    const tableRow = document.createElement("tr");
    const col1 = document.createElement("td");
    col1.textContent = "Required Labels (Missing)";
    const col2 = document.createElement("td");
    col2.textContent = "Forbidden Labels (Shouldn't have)";
    const col3 = document.createElement("td");

    const body = document.createElement("tbody");
    queries.forEach((query) => {
        const row = document.createElement("tr");
        row.appendChild(createMergeLabelCell(query.labels, true));
        row.appendChild(createMergeLabelCell(query.missingLabels));
        row.appendChild(createQueryDetailsBtn(query));
        body.appendChild(row);
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
 * Creates the merge label requirement status.
 */
function createMergeLabelStatus(prLabels: Array<{Label: Label}> = [], queries: ProcessedQuery[]): HTMLElement {
    const statusContainer = document.createElement("div");
    statusContainer.classList.add("status-container");
    const status = document.createElement("div");
    statusContainer.appendChild(status);
    if (queries.length > 0) {
        const labelConflict = !hasResolvedLabels(queries[0]);
        if (labelConflict) {
            status.appendChild(createIcon("error", "", ["status-icon", "failed"]));
            status.appendChild(document.createTextNode("Does not meet label requirements"));
            // Creates help button
            const iconButton = createIcon("help", "", ["help-icon-button"], true);
            status.appendChild(iconButton);
            // Shows dialog
            const dialog = document.querySelector("#merge-help-dialog") as HTMLDialogElement;
            iconButton.addEventListener("click", (event) => {
                dialog.showModal();
                event.stopPropagation();
            });
        } else {
            status.appendChild(createIcon("check_circle", "", ["status-icon", "succeeded"]));
            status.appendChild(document.createTextNode("Meets label requirements"));
        }

        const arrowIcon = createIcon("expand_less");
        arrowIcon.classList.add("arrow-icon");

        status.classList.add("status", "expandable");
        status.appendChild(arrowIcon);

        const queriesTable = createQueriesTable(prLabels, queries);
        if (!labelConflict) {
            queriesTable.classList.add("hidden");
            arrowIcon.textContent = "expand_more";
        }
        status.addEventListener("click", () => {
            queriesTable.classList.toggle("hidden");
            if (queriesTable.classList.contains("hidden")) {
                const offLabels = queriesTable.querySelectorAll(
                    ".merge-table-label.off");
                offLabels.forEach((offLabel) => {
                    offLabel.classList.add("hidden");
                });
            }
            arrowIcon.textContent = arrowIcon.textContent === "expand_more"
                ? "expand_less" : "expand_more";
        });
        statusContainer.appendChild(queriesTable);
    } else {
        status.appendChild(document.createTextNode("No Tide query found"));
        status.classList.add("no-status");
        const p = document.createElement("P");
        p.textContent = "This repo may not be configured to use Tide.";
        status.appendChild(createStatusHelp("Tide query not found", [p]));
    }
    return statusContainer;
}

/**
 * Creates the merge conflict status.
 */
function createMergeConflictStatus(mergeConflict: boolean): HTMLElement {
    const statusContainer = document.createElement("div");
    statusContainer.classList.add("status-container");
    const status = document.createElement("div");
    if (mergeConflict) {
        status.appendChild(createIcon("error", "", ["status-icon", "failed"]));
        status.appendChild(
            document.createTextNode("Has merge conflicts"));
    } else {
        status.appendChild(
            createIcon("check_circle", "", ["status-icon", "succeeded"]));
        status.appendChild(
            document.createTextNode("Does not appear to have merge conflicts"));
    }
    status.classList.add("status");
    statusContainer.appendChild(status);
    return statusContainer;
}

/**
 * Creates a help button on the status.
 */
function createStatusHelp(title: string, content: HTMLElement[]): HTMLElement {
    const dialog = document.querySelector("#status-help-dialog")! as HTMLDialogElement;
    const dialogTitle = dialog.querySelector(".mdl-dialog__title")!;
    const dialogContent = dialog.querySelector(".mdl-dialog__content")!;
    const helpIcon = createIcon("help", "", ["help-icon-button"], true);
    helpIcon.addEventListener("click", (event) => {
        dialogTitle.textContent = title;
        while (dialogContent.firstChild) {
            dialogContent.removeChild(dialogContent.firstChild);
        }
        content.forEach((el) => {
            dialogContent.appendChild(el);
        });
        dialog.showModal();
        event.stopPropagation();
    });

    return helpIcon;
}

/**
 * Creates a generic conflict status.
 */
function createGenericConflictStatus(pr: PullRequest, hasConflict: boolean, message: string): HTMLElement {
    const statusContainer = document.createElement("div");
    statusContainer.classList.add("status-container");
    const status = document.createElement("div");
    if (hasConflict) {
        status.appendChild(createIcon("error", "", ["status-icon", "failed"]));
        status.appendChild(
            document.createTextNode(message));
        status.classList.add("status");
        statusContainer.appendChild(status);
    }
    return statusContainer;
}

function createPRCardBody(pr: PullRequest, builds: UnifiedContext[], queries: ProcessedQuery[],
                          mergeable: boolean, branchConflict: boolean,
                          authorConflict: boolean, milestoneConflict: boolean): HTMLElement {
    const cardBody = document.createElement("div");
    const title = document.createElement("h3");
    title.textContent = pr.Title;

    cardBody.classList.add("mdl-card__supporting-text");
    cardBody.appendChild(title);
    cardBody.appendChild(createJobStatus(builds));
    const nodes = pr.Labels && pr.Labels.Nodes ? pr.Labels.Nodes : [];
    cardBody.appendChild(createMergeLabelStatus(nodes, queries));
    cardBody.appendChild(createMergeConflictStatus(mergeable));
    cardBody.appendChild(createGenericConflictStatus(pr, branchConflict, `Merging into branch ${pr.BaseRef.Name} is currently forbidden`));
    if (queries.length) {
        cardBody.appendChild(createGenericConflictStatus(pr, authorConflict, `Only merges with author ${queries[0].author} are currently allowed`));
        cardBody.appendChild(createGenericConflictStatus(pr, milestoneConflict, `Only merges into milestone ${queries[0].milestone} are currently allowed`));
    }
    return cardBody;
}

/**
 * Compare function that prioritizes jobs which are in failure state.
 */
function compareJobFn(a: UnifiedContext, b: UnifiedContext): number {
    const stateToPrio: {[key: string]: number} = {};
    stateToPrio.success = stateToPrio.expected = 3;
    stateToPrio.aborted = 2;
    stateToPrio.pending = stateToPrio.triggered = 1;
    stateToPrio.error = stateToPrio.failure = 0;

    return stateToPrio[a.state] > stateToPrio[b.state] ? 1
        : stateToPrio[a.state] < stateToPrio[b.state] ? -1 : 0;
}

/**
 * closestMatchingQueries returns a list of processed TideQueries that match the PR in descending order of likeliness.
 */
function closestMatchingQueries(pr: PullRequest, queries: TideQuery[]): ProcessedQuery[] {
    const prLabelsSet = new Set();
    if (pr.Labels && pr.Labels.Nodes) {
        pr.Labels.Nodes.forEach((label) => {
            prLabelsSet.add(label.Label.Name);
        });
    }
    const processedQueries: ProcessedQuery[] = [];
    queries.forEach((query) => {
        let score = 0.0;
        const labels: ProcessedLabel[] = [];
        const missingLabels: ProcessedLabel[] = [];
        (query.labels || []).sort((a, b) => {
            if (a.length === b.length) {
                return 0;
            }
            return a.length < b.length ? -1 : 1;
        });
        (query.missingLabels || []).sort((a, b) => {
            if (a.length === b.length) {
                return 0;
            }
            return a.length < b.length ? -1 : 1;
        });
        (query.labels || []).forEach((label) => {
            labels.push({name: label, own: prLabelsSet.has(label)});
            score += labels[labels.length - 1].own ? 1 : 0;
        });
        (query.missingLabels || []).forEach((label) => {
            missingLabels.push({name: label, own: prLabelsSet.has(label)});
            score += missingLabels[missingLabels.length - 1].own ? 0 : 1;
        });
        score = (labels.length + missingLabels.length > 0) ? score
            / (labels.length + missingLabels.length) : 1.0;
        processedQueries.push({
            author: query.author,
            excludedBranches: query.excludedBranches,
            includedBranches: query.includedBranches,
            labels,
            milestone: query.milestone,
            missingLabels,
            score,
        });
    });
    // Sort queries by descending score order.
    processedQueries.sort((q1, q2) => {
        if (pr.BaseRef && pr.BaseRef.Name) {
            let q1Excluded = 0;
            let q2Excluded = 0;
            if (q1.excludedBranches && q1.excludedBranches.indexOf(pr.BaseRef.Name) !== -1) {
                q1Excluded = 1;
            }
            if (q2.excludedBranches && q2.excludedBranches.indexOf(pr.BaseRef.Name) !== -1) {
                q2Excluded = -1;
            }
            if (q1Excluded + q2Excluded !== 0) {
                return q1Excluded + q2Excluded;
            }

            let q1Included = 0;
            let q2Included = 0;
            if (q1.includedBranches && q1.includedBranches.indexOf(pr.BaseRef.Name) !== -1) {
                q1Included = -1;
            }
            if (q2.includedBranches && q2.includedBranches.indexOf(pr.BaseRef.Name) !== -1) {
                q2Included = 1;
            }
            if (q1Included + q2Included !== 0) {
                return q1Included + q2Included;
            }
        }

        const prAuthor = pr.Author && normLogin(pr.Author.Login);
        const q1Author = normLogin(q1.author);
        const q2Author = normLogin(q2.author);

        if (prAuthor && q1Author !== q2Author) {
            if (q1.author && prAuthor === q1Author) {
                return -1;
            }
            if (q2.author && prAuthor === q2Author) {
                return 1;
            }
        }
        if (pr.Milestone && pr.Milestone.Title && q1.milestone !== q2.milestone) {
            if (q1.milestone && pr.Milestone.Title === q1.milestone) {
                return -1;
            }
            if (q2.milestone && pr.Milestone.Title === q2.milestone) {
                return 1;
            }
        }
        if (Math.abs(q1.score - q2.score) < Number.EPSILON) {
            return 0;
        }
        return q1.score > q2.score ? -1 : 1;
    });
    return processedQueries;
}

/**
 * Normalizes GitHub login strings
 */
function normLogin(login: string = ""): string {
    return login.toLowerCase().replace(/^@/, "");
}

/**
 * Creates a PR card.
 */
function createPRCard(pr: PullRequest, builds: UnifiedContext[] = [], queries: ProcessedQuery[] = [], tidePools: TidePool[] = []): HTMLElement {
    const prCard = document.createElement("div");
    // jobs need to be sorted from high priority (failure, error) to low
    // priority (success)
    builds.sort(compareJobFn);
    prCard.classList.add("pr-card", "mdl-card");
    const hasMatchingQuery = queries.length > 0;
    const mergeConflict = pr.Mergeable ? pr.Mergeable === "CONFLICTING" : false;
    const branchConflict = !!((pr.BaseRef && pr.BaseRef.Name && hasMatchingQuery) &&
        ((queries[0].excludedBranches && queries[0].excludedBranches!.indexOf(pr.BaseRef.Name) !== -1) ||
            (queries[0].includedBranches && queries[0].includedBranches!.indexOf(pr.BaseRef.Name) === -1)));
    const authorConflict = hasMatchingQuery && queries[0].author ? (!pr.Author || !pr.Author.Login || normLogin(pr.Author.Login) !== normLogin(queries[0].author)) : false;
    const milestoneConflict = hasMatchingQuery && queries[0].milestone ? (!pr.Milestone || !pr.Milestone.Title || pr.Milestone.Title !== queries[0].milestone) : false;
    const labelConflict = hasMatchingQuery ? !hasResolvedLabels(queries[0]) : false;
    prCard.appendChild(createPRCardTitle(pr, tidePools, jobVagueState(builds), !hasMatchingQuery, labelConflict, mergeConflict, branchConflict, authorConflict, milestoneConflict));
    prCard.appendChild(createPRCardBody(pr, builds, queries, mergeConflict, branchConflict, authorConflict, milestoneConflict));
    return prCard;
}

/**
 * Redirect to initiate github login flow.
 */
function forceGitHubLogin(): void {
    window.location.href = window.location.origin + `/github-login?dest=${relativeURL()}`;
}

type VagueState = "succeeded" | "failed" | "pending" | "unknown";

/**
 * Returns the job status based on its state.
 */
function jobVagueState(builds: UnifiedContext[]): VagueState {
    if (builds.length === 0) {
        return "unknown";
    }
    switch (builds[0].state) {
        case "success":
        case "expected":
            return "succeeded";
        case "failure":
        case "error":
            return "failed";
        default:
            return "pending";
    }
}

/**
 * Returns -1 if there is no query. 1 if the PR is able to be merged by checking
 * the score of the first query in the query list (score === 1), the list has
 * been sorted by scores, otherwise 0.
 * @param query
 * @return {boolean}
 */
function hasResolvedLabels(query: ProcessedQuery): boolean {
    return Math.abs(query.score - 1.0) < Number.EPSILON;
}

/**
 * Returns an icon element.
 */
function createIcon(iconString: string, tooltip?: string, styles?: string[], isButton?: true): HTMLButtonElement;
function createIcon(iconString: string, tooltip = "", styles: string[] = [], isButton = false): HTMLElement {
    const icon = document.createElement("i");
    icon.classList.add("icon-button", "material-icons");
    icon.textContent = iconString;
    if (tooltip !== "") {
        icon.title = tooltip;
    }
    if (!isButton) {
        icon.classList.add(...styles);
        return icon;
    }
    const container = document.createElement("button");
    container.appendChild(icon);
    container.classList.add("mdl-button", "mdl-js-button", "mdl-button--icon",
        ...styles);

    return container;
}

/**
 * Create a simple message with an icon.
 */
function createMessage(msg: string, icStr?: string): HTMLElement {
    const el = document.createElement("h3");
    el.textContent = msg;
    if (icStr) {
        const ic = createIcon(icStr, "", ["message-icon"]);
        el.appendChild((ic));
    }
    const msgContainer = document.createElement("div");
    msgContainer.appendChild(el);
    msgContainer.classList.add("message");

    return msgContainer;
}
