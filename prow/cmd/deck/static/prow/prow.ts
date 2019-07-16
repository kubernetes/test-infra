import moment from "moment";
import {Job, JobState, JobType} from "../api/prow";
import {cell, getCookieByName, icon} from "../common/common";
import {getParameterByName} from "../common/urls";
import {FuzzySearch} from './fuzzy-search';
import {JobHistogram, JobSample} from './histogram';

declare const allBuilds: Job[];
declare const spyglass: boolean;
declare const rerunCreatesJob: boolean;
declare const csrfToken: string;

function shortenBuildRefs(buildRef: string): string {
    return buildRef && buildRef.replace(/:[0-9a-f]*/g, '');
}

interface RepoOptions {
    types: {[key: string]: boolean};
    repos: {[key: string]: boolean};
    jobs: {[key: string]: boolean};
    authors: {[key: string]: boolean};
    pulls: {[key: string]: boolean};
    batches: {[key: string]: boolean};
    states: {[key: string]: boolean};
}

function optionsForRepo(repo: string): RepoOptions {
    const opts: RepoOptions = {
        authors: {},
        batches: {},
        jobs: {},
        pulls: {},
        repos: {},
        states: {},
        types: {},
    };

    for (const build of allBuilds) {
        opts.types[build.type] = true;
        const repoKey = `${build.refs.org}/${build.refs.repo}`;
        if (repoKey) {
            opts.repos[repoKey] = true;
        }
        if (!repo || repo === repoKey) {
            opts.jobs[build.job] = true;
            opts.states[build.state] = true;
            if (build.type === "presubmit" &&
                build.refs.pulls &&
                build.refs.pulls.length > 0) {
                opts.authors[build.refs.pulls[0].author] = true;
                opts.pulls[build.refs.pulls[0].number] = true;
            } else if (build.type === "batch") {
                opts.batches[shortenBuildRefs(build.refs_key)] = true;
            }
        }
    }

    return opts;
}

function redrawOptions(fz: FuzzySearch, opts: RepoOptions) {
    const ts = Object.keys(opts.types).sort();
    const selectedType = addOptions(ts, "type") as JobType;
    const rs = Object.keys(opts.repos).filter((r) => r !== "/").sort();
    addOptions(rs, "repo");
    const js = Object.keys(opts.jobs).sort();
    const jobInput = document.getElementById("job-input") as HTMLInputElement;
    const jobList = document.getElementById("job-list") as HTMLUListElement;
    addOptionFuzzySearch(fz, js, "job", jobList, jobInput);
    const as = Object.keys(opts.authors).sort(
        (a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
    addOptions(as, "author");
    if (selectedType === "batch") {
        opts.pulls = opts.batches;
    }
    if (selectedType !== "periodic" && selectedType !== "postsubmit") {
        const ps = Object.keys(opts.pulls).sort((a, b) => Number(a) - Number(b));
        addOptions(ps, "pull");
    } else {
        addOptions([], "pull");
    }
    const ss = Object.keys(opts.states).sort();
    addOptions(ss, "state");
}

function adjustScroll(el: Element): void {
    const parent = el.parentElement!;
    const parentRect = parent.getBoundingClientRect();
    const elRect = el.getBoundingClientRect();

    if (elRect.top < parentRect.top) {
        parent.scrollTop -= elRect.height;
    } else if (elRect.top + elRect.height >= parentRect.top
        + parentRect.height) {
        parent.scrollTop += elRect.height;
    }
}

function handleDownKey(): void {
    const activeSearches =
        document.getElementsByClassName("active-fuzzy-search");
    if (activeSearches !== null && activeSearches.length !== 1) {
        return;
    }
    const activeSearch = activeSearches[0];
    if (activeSearch.tagName !== "UL" ||
        activeSearch.childElementCount === 0) {
        return;
    }

    const selectedJobs = activeSearch.getElementsByClassName("job-selected");
    if (selectedJobs.length > 1) {
        return;
    }
    if (selectedJobs.length === 0) {
        // If no job selected, select the first one that visible in the list.
        const jobs = Array.from(activeSearch.children)
            .filter((elChild) => {
                const childRect = elChild.getBoundingClientRect();
                const listRect = activeSearch.getBoundingClientRect();
                return childRect.top >= listRect.top &&
                    (childRect.top < listRect.top + listRect.height);
            });
        if (jobs.length === 0) {
            return;
        }
        jobs[0].classList.add("job-selected");
        return;
    }
    const selectedJob = selectedJobs[0] as Element;
    const nextSibling = selectedJob.nextElementSibling;
    if (!nextSibling) {
        return;
    }

    selectedJob.classList.remove("job-selected");
    nextSibling.classList.add("job-selected");
    adjustScroll(nextSibling);
}

function handleUpKey(): void {
    const activeSearches =
        document.getElementsByClassName("active-fuzzy-search");
    if (activeSearches && activeSearches.length !== 1) {
        return;
    }
    const activeSearch = activeSearches[0];
    if (activeSearch.tagName !== "UL" ||
        activeSearch.childElementCount === 0) {
        return;
    }

    const selectedJobs = activeSearch.getElementsByClassName("job-selected");
    if (selectedJobs.length !== 1) {
        return;
    }

    const selectedJob = selectedJobs[0] as Element;
    const previousSibling = selectedJob.previousElementSibling;
    if (!previousSibling) {
        return;
    }

    selectedJob.classList.remove("job-selected");
    previousSibling.classList.add("job-selected");
    adjustScroll(previousSibling);
}

window.onload = (): void => {
    const topNavigator = document.getElementById("top-navigator")!;
    let navigatorTimeOut: number | undefined;
    const main = document.querySelector("main")! as HTMLElement;
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

    document.addEventListener("keydown", (event) => {
        if (event.keyCode === 40) {
            handleDownKey();
        } else if (event.keyCode === 38) {
            handleUpKey();
        }
    });
    // Register selection on change functions
    const filterBox = document.getElementById("filter-box")!;
    const options = filterBox.querySelectorAll("select")!;
    options.forEach((opt) => {
        opt.onchange = () => {
            redraw(fz);
        };
    });
    // Attach job status bar on click
    const stateFilter = document.getElementById("state")! as HTMLSelectElement;
    document.querySelectorAll(".job-bar-state").forEach((jb) => {
        const state = jb.id.slice("job-bar-".length);
        if (state === "unknown") {
            return;
        }
        jb.addEventListener("click", () => {
            stateFilter.value = state;
            stateFilter.onchange!.call(stateFilter, new Event("change"));
        });
    });
    // Attach job histogram on click to scroll the selected build into view
    const jobHistogram = document.getElementById("job-histogram-content") as HTMLTableSectionElement;
    jobHistogram.addEventListener("click", (event) => {
        const target = event.target as HTMLElement;
        if (target == null) {
            return;
        }
        if (!target.classList.contains('active')) {
            return;
        }
        const row = target.dataset.sampleRow;
        if (row == null || row.length === 0) {
            return;
        }
        const rowNumber = Number(row);
        const builds = document.getElementById("builds")!.getElementsByTagName("tbody")[0];
        if (builds == null || rowNumber >= builds.childNodes.length) {
            return;
        }
        const targetRow = builds.childNodes[rowNumber] as HTMLTableRowElement;
        targetRow.scrollIntoView();
    });
    // set dropdown based on options from query string
    const opts = optionsForRepo("");
    const fz = initFuzzySearch(
        "job",
        "job-input",
        "job-list",
        Object.keys(opts.jobs).sort());
    redrawOptions(fz, opts);
    redraw(fz);
};

function displayFuzzySearchResult(el: HTMLElement, inputContainer: ClientRect | DOMRect): void {
    el.classList.add("active-fuzzy-search");
    el.style.top = inputContainer.height - 1 + "px";
    el.style.width = inputContainer.width + "px";
    el.style.height = 200 + "px";
    el.style.zIndex = "9999";
}

function fuzzySearch(fz: FuzzySearch, id: string, list: HTMLElement, input: HTMLInputElement): void {
    const inputValue = input.value.trim();
    addOptionFuzzySearch(fz, fz.search(inputValue), id, list, input, true);
}

function validToken(token: number): boolean {
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

function handleEnterKeyDown(fz: FuzzySearch, list: HTMLElement, input: HTMLInputElement): void {
    const selectedJobs = list.getElementsByClassName("job-selected");
    if (selectedJobs && selectedJobs.length === 1) {
        input.value = (selectedJobs[0] as HTMLElement).innerHTML;
    }
    // TODO(@qhuynh96): according to discussion in https://github.com/kubernetes/test-infra/pull/7165, the
    // fuzzy search should respect user input no matter it is in the list or not. User may
    // experience being redirected back to default view if the search input is invalid.
    input.blur();
    list.classList.remove("active-fuzzy-search");
    redraw(fz);
}

function registerFuzzySearchHandler(fz: FuzzySearch, id: string, list: HTMLElement, input: HTMLInputElement): void {
    input.addEventListener("keydown", (event) => {
        if (event.keyCode === 13) {
            handleEnterKeyDown(fz, list, input);
        } else if (validToken(event.keyCode)) {
            // Delay 1 frame that the input character is recorded before getting
            // input value
            setTimeout(() => fuzzySearch(fz, id, list, input), 32);
        }
    });
}

function initFuzzySearch(id: string, inputId: string, listId: string,
                         data: string[]): FuzzySearch {
    const fz = new FuzzySearch(data);
    const el = document.getElementById(id)!;
    const input = document.getElementById(inputId)! as HTMLInputElement;
    const list = document.getElementById(listId)!;

    list.classList.remove("active-fuzzy-search");
    input.addEventListener("focus", () => {
        fuzzySearch(fz, id, list, input);
        displayFuzzySearchResult(list, el.getBoundingClientRect());
    });
    input.addEventListener("blur", () => list.classList.remove("active-fuzzy-search"));

    registerFuzzySearchHandler(fz, id, list, input);
    return fz;
}

function registerJobResultEventHandler(fz: FuzzySearch, li: HTMLElement, input: HTMLInputElement) {
    li.addEventListener("mousedown", (event) => {
        input.value = (event.currentTarget as HTMLElement).innerHTML;
        redraw(fz);
    });
    li.addEventListener("mouseover", (event) => {
        const selectedJobs = document.getElementsByClassName("job-selected");
        if (!selectedJobs) {
            return;
        }

        for (const job of Array.from(selectedJobs)) {
            job.classList.remove("job-selected");
        }
        (event.currentTarget as HTMLElement).classList.add("job-selected");
    });
    li.addEventListener("mouseout", (event) => {
        (event.currentTarget as HTMLElement).classList.remove("job-selected");
    });
}

function addOptionFuzzySearch(fz: FuzzySearch, data: string[], id: string,
                              list: HTMLElement, input: HTMLInputElement,
                              stopAutoFill?: boolean): void {
    if (!stopAutoFill) {
        input.value = getParameterByName(id) || '';
    }
    while (list.firstChild) {
        list.removeChild(list.firstChild);
    }
    list.scrollTop = 0;
    for (const datum of data) {
        const li = document.createElement("li");
        li.innerHTML = datum;
        registerJobResultEventHandler(fz, li, input);
        list.appendChild(li);
    }
}

function addOptions(options: string[], selectID: string): string | undefined {
    const sel = document.getElementById(selectID)! as HTMLSelectElement;
    while (sel.length > 1) {
        sel.removeChild(sel.lastChild!);
    }
    const param = getParameterByName(selectID);
    for (const option of options) {
        const o = document.createElement("option");
        o.text = option;
        if (param && option === param) {
            o.selected = true;
        }
        sel.appendChild(o);
    }
    return param;
}

function selectionText(sel: HTMLSelectElement): string {
    return sel.selectedIndex === 0 ? "" : sel.options[sel.selectedIndex].text;
}

function equalSelected(sel: string, t: string): boolean {
    return sel === "" || sel === t;
}

function groupKey(build: Job): string {
    const pr = (build.refs.pulls && build.refs.pulls.length === 1) ? build.refs.pulls[0].number : 0;
    return `${build.refs.repo} ${pr} ${build.refs_key}`;
}

// escapeRegexLiteral ensures the given string is escaped so that it is treated as
// an exact value when used within a RegExp. This is the standard substitution recommended
// by https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/Regular_Expressions.
function escapeRegexLiteral(s: string): string {
    return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function redraw(fz: FuzzySearch): void {
    const rerunStatus = getParameterByName("rerun");
    const modal = document.getElementById('rerun')!;
    const rerunCommand = document.getElementById('rerun-content')!;
    window.onclick = (event) => {
        if (event.target === modal) {
            modal.style.display = "none";
        }
    };
    const builds = document.getElementById("builds")!.getElementsByTagName(
        "tbody")[0];
    while (builds.firstChild) {
        builds.removeChild(builds.firstChild);
    }

    const args: string[] = [];

    function getSelection(name: string): string {
        const sel = selectionText(document.getElementById(name) as HTMLSelectElement);
        if (sel && opts && !opts[name + 's' as keyof RepoOptions][sel]) {
            return "";
        }
        if (sel !== "") {
            args.push(`${name}=${encodeURIComponent(sel)}`);
        }
        return sel;
    }

    function getSelectionFuzzySearch(id: string, inputId: string): RegExp {
        const input = document.getElementById(inputId) as HTMLInputElement;
        const inputText = input.value;
        if (inputText === "") {
            return new RegExp('');
        }
        if (inputText !== "") {
            args.push(`${id}=${encodeURIComponent(inputText)}`);
        }
        if (inputText !== "" && opts && opts[id + 's' as keyof RepoOptions][inputText]) {
            return new RegExp(`^${escapeRegexLiteral(inputText)}$`);
        }
        const expr = inputText.split('*').map(escapeRegexLiteral);
        return new RegExp(`^${expr.join('.*')}$`);
    }

    const repoSel = getSelection("repo");
    const opts = optionsForRepo(repoSel);

    const typeSel = getSelection("type") as JobType;
    if (typeSel === "batch") {
        opts.pulls = opts.batches;
    }
    const pullSel = getSelection("pull");
    const authorSel = getSelection("author");
    const jobSel = getSelectionFuzzySearch("job", "job-input");
    const stateSel = getSelection("state");

    if (window.history && window.history.replaceState !== undefined) {
        if (args.length > 0) {
            history.replaceState(null, "", "/?" + args.join('&'));
        } else {
            history.replaceState(null, "", "/");
        }
    }
    fz.setDict(Object.keys(opts.jobs));
    redrawOptions(fz, opts);

    let lastKey = '';
    const jobCountMap = new Map() as Map<JobState, number>;
    const jobInterval: Array<[number, number]> = [[3600 * 3, 0], [3600 * 12, 0], [3600 * 48, 0]];
    let currentInterval = 0;
    const jobHistogram = new JobHistogram();
    const now = moment().unix();
    let totalJob = 0;
    let displayedJob = 0;
    for (let i = 0; i < allBuilds.length; i++) {
        const build = allBuilds[i];
        if (!equalSelected(typeSel, build.type)) {
            continue;
        }
        if (!equalSelected(repoSel, `${build.refs.org}/${build.refs.repo}`)) {
            continue;
        }
        if (!equalSelected(stateSel, build.state)) {
            continue;
        }
        if (!jobSel.test(build.job)) {
            continue;
        }
        if (build.type === "presubmit") {
            if (build.refs.pulls && build.refs.pulls.length > 0) {
                const pull = build.refs.pulls[0];
                if (!equalSelected(pullSel, pull.number.toString())) {
                    continue;
                }
                if (!equalSelected(authorSel, pull.author)) {
                    continue;
                }
            }
        } else if (build.type === "batch" && !authorSel) {
            if (!equalSelected(pullSel, shortenBuildRefs(build.refs_key))) {
                continue;
            }
        } else if (pullSel || authorSel) {
            continue;
        }

        if (!jobCountMap.has(build.state)) {
            jobCountMap.set(build.state, 0);
        }
        totalJob++;
        jobCountMap.set(build.state, jobCountMap.get(build.state)! + 1);

        // accumulate a count of the percentage of successful jobs over each interval
        const started = Number(build.started);
        if (currentInterval >= 0 && (now - started) > jobInterval[currentInterval][0]) {
            let successCount = jobCountMap.get("success");
            if (!successCount) {
                successCount = 0;
            }
            let failureCount = jobCountMap.get("failure");
            if (!failureCount) {
                failureCount = 0;
            }
            const total = successCount + failureCount;
            if (total > 0) {
                jobInterval[currentInterval][1] = successCount / total;
            } else {
                jobInterval[currentInterval][1] = 0;
            }
            currentInterval++;
            if (currentInterval >= jobInterval.length) {
                currentInterval = -1;
            }
        }

        if (displayedJob >= 500) {
            jobHistogram.add(new JobSample(started, parseDuration(build.duration), build.state, -1));
            continue;
        } else {
            jobHistogram.add(new JobSample(started, parseDuration(build.duration), build.state, builds.childElementCount));
        }
        displayedJob++;
        const r = document.createElement("tr");
        r.appendChild(cell.state(build.state));
        if (build.pod_name) {
            const logIcon = icon.create("description", "Build log");
            logIcon.href = `log?job=${build.job}&id=${build.build_id}`;
            const c = document.createElement("td");
            c.classList.add("icon-cell");
            c.appendChild(logIcon);
            r.appendChild(c);
        } else {
            r.appendChild(cell.text(""));
        }
        r.appendChild(createRerunCell(modal, rerunCommand, build.prow_job));
        r.appendChild(createViewJobCell(build.prow_job));
        const key = groupKey(build);
        if (key !== lastKey) {
            // This is a different PR or commit than the previous row.
            lastKey = key;
            r.className = "changed";

            if (build.type === "periodic") {
                r.appendChild(cell.text(""));
            } else {
                let repoLink = build.refs.repo_link;
                if (!repoLink) {
                    repoLink = `https://github.com/${build.refs.org}/${build.refs.repo}`;
                }
                r.appendChild(cell.link(`${build.refs.org}/${build.refs.repo}`, repoLink));
            }
            if (build.type === "presubmit") {
                if (build.refs.pulls && build.refs.pulls.length > 0) {
                    r.appendChild(cell.prRevision(`${build.refs.org}/${build.refs.repo}`, build.refs.pulls[0]));
                } else {
                    r.appendChild(cell.text(""));
                }
            } else if (build.type === "batch") {
                r.appendChild(batchRevisionCell(build));
            } else if (build.type === "postsubmit") {
                r.appendChild(cell.commitRevision(`${build.refs.org}/${build.refs.repo}`, build.refs.base_ref || "",
                    build.refs.base_sha || "", build.refs.base_link || ""));
            } else if (build.type === "periodic") {
                r.appendChild(cell.text(""));
            }
        } else {
            // Don't render identical cells for the same PR/commit.
            r.appendChild(cell.text(""));
            r.appendChild(cell.text(""));
        }
        if (spyglass) {
            const buildIndex = build.url.indexOf('/build/');
            if (buildIndex !== -1) {
                const url = `${window.location.origin}/view/gcs/${build.url.substring(buildIndex + '/build/'.length)}`;
                r.appendChild(createSpyglassCell(url));
            } else if (build.url.includes('/view/')) {
                r.appendChild(createSpyglassCell(build.url));
            } else {
                r.appendChild(cell.text(''));
            }
        } else {
            r.appendChild(cell.text(''));
        }
        if (build.url === "") {
            r.appendChild(cell.text(build.job));
        } else {
            r.appendChild(cell.link(build.job, build.url));
        }

        r.appendChild(cell.time(i.toString(), moment.unix(Number(build.started))));
        r.appendChild(cell.text(build.duration));
        builds.appendChild(r);
    }

    // fill out the remaining intervals if necessary
    if (currentInterval !== -1) {
        let successCount = jobCountMap.get("success");
        if (!successCount) {
            successCount = 0;
        }
        let failureCount = jobCountMap.get("failure");
        if (!failureCount) {
            failureCount = 0;
        }
        const total = successCount + failureCount;
        for (let i = currentInterval; i < jobInterval.length; i++) {
            if (total > 0) {
                jobInterval[i][1] = successCount / total;
            } else {
                jobInterval[i][1] = 0;
            }
        }
    }

    const jobSummary = document.getElementById("job-histogram-summary")!;
    const success = jobInterval.map((interval) => {
        if (interval[1] < 0.5) {
            return `${formatDuration(interval[0])}: <span class="state failure">${Math.ceil(interval[1] * 100)}%</span>`;
        }
        return `${formatDuration(interval[0])}: <span class="state success">${Math.ceil(interval[1] * 100)}%</span>`;
    }).join(", ");
    jobSummary.innerHTML = `Success rate over time: ${success}`;
    const jobCount = document.getElementById("job-count")!;
    jobCount.textContent = `Showing ${displayedJob}/${totalJob} jobs`;
    drawJobBar(totalJob, jobCountMap);

    // if we aren't filtering the output, cap the histogram y axis to 2 hours because it
    // contains the bulk of our jobs
    let max = Number.MAX_SAFE_INTEGER;
    if (totalJob === allBuilds.length) {
        max = 2 * 3600;
    }
    drawJobHistogram(totalJob, jobHistogram, now - (12 * 3600), now, max);
    if (rerunStatus === "gh_redirect") {
        modal.style.display = "block";
        rerunCommand.innerHTML = "Rerunning that job requires GitHub login. Now that you're logged in, try again";
    }
}

function createRerunCell(modal: HTMLElement, rerunElement: HTMLElement, prowjob: string): HTMLTableDataCellElement {
    const url = `${location.protocol}//${location.host}/rerun?prowjob=${prowjob}`;
    const c = document.createElement("td");
    const i = icon.create("refresh", "Show instructions for rerunning this job");
    const login = getCookieByName("access-token-session");
    i.onclick = () => {
        modal.style.display = "block";
        rerunElement.innerHTML = `kubectl create -f "<a href="${url}">${url}</a>"`;
        const copyButton = document.createElement('a');
        copyButton.className = "mdl-button mdl-js-button mdl-button--icon";
        copyButton.onclick = () => copyToClipboardWithToast(`kubectl create -f "${url}"`);
        copyButton.innerHTML = "<i class='material-icons state triggered' style='color: gray'>file_copy</i>";
        rerunElement.appendChild(copyButton);
        if (rerunCreatesJob) {
            const runButton = document.createElement('a');
            runButton.innerHTML = "<button class='mdl-button mdl-js-button'>Rerun</button>";
            if (login === "") {
                runButton.href = `/github-login?dest=%2F?rerun=gh_redirect`;
            } else {
                runButton.onclick = async () => {
                    gtag("event", "troll_rerun_popup", {
                        event_category: "engagement",
                        transport_type: "beacon",
                    });
                    const result = await fetch(url, {
                        headers: {
                            "Content-type": "application/x-www-form-urlencoded; charset=UTF-8",
                            "X-CSRF-Token": csrfToken,
                        },
                        method: 'post',
                    });
                    const data = await result.text();
                    rerunElement.innerHTML = data;
                };
            }
            rerunElement.appendChild(runButton);
        }
    };
    c.appendChild(i);
    c.classList.add("icon-cell");
    return c;
}

function createViewJobCell(prowjob: string): HTMLTableDataCellElement {
    const c = document.createElement("td");
    const i = icon.create("pageview", "Show job YAML", () => gtag("event", "view_job_yaml", {event_category: "engagement", transport_type: "beacon"}));
    i.href = `/prowjob?prowjob=${prowjob}`;
    c.classList.add("icon-cell");
    c.appendChild(i);
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
function copyToClipboard(text: string) {
    if (window.clipboardData && window.clipboardData.setData) {
        // IE specific code path to prevent textarea being shown while dialog is visible.
        return window.clipboardData.setData("Text", text);
    } else if (document.queryCommandSupported && document.queryCommandSupported("copy")) {
        const textarea = document.createElement("textarea");
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

function copyToClipboardWithToast(text: string): void {
    copyToClipboard(text);

    const toast = document.getElementById("toast") as SnackbarElement<HTMLDivElement>;
    toast.MaterialSnackbar.showSnackbar({message: "Copied to clipboard"});
}

function batchRevisionCell(build: Job): HTMLTableDataCellElement {
    const c = document.createElement("td");
    if (!build.refs.pulls) {
        return c;
    }
    for (let i = 0; i < build.refs.pulls.length; i++) {
        if (i !== 0) {
            c.appendChild(document.createTextNode(", "));
        }
        const l = document.createElement("a");
        const link = build.refs.pulls[i].link;
        if (link) {
            l.href = link;
        } else {
            l.href = `https://github.com/${build.refs.org}/${build.refs.repo}/pull/${build.refs.pulls[i].number}`;
        }
        l.text = build.refs.pulls[i].number.toString();
        c.appendChild(document.createTextNode("#"));
        c.appendChild(l);
    }
    return c;
}

function drawJobBar(total: number, jobCountMap: Map<JobState, number>): void {
  const states: JobState[] = ["success", "pending", "triggered", "error", "failure", "aborted", ""];
  states.sort((s1, s2) => {
    return jobCountMap.get(s1)! - jobCountMap.get(s2)!;
  });
  states.forEach((state, index) => {
    const count = jobCountMap.get(state);
    // If state is undefined or empty, treats it as unknown state.
    if (!state) {
      state = "unknown";
    }
    const id = "job-bar-" + state;
    const el = document.getElementById(id)!;
    const tt = document.getElementById(state + "-tooltip")!;
    if (!count || count === 0 || total === 0) {
      el.textContent = "";
      tt.textContent = "";
      el.style.width = "0";
    } else {
      el.textContent = count.toString();
      tt.textContent = `${count} ${stateToAdj(state)} jobs`;
      if (index === states.length - 1) {
        el.style.width = "auto";
      } else {
        el.style.width = Math.max((count / total * 100), 1) + "%";
      }
    }
  });
}

function stateToAdj(state: JobState): string {
    switch (state) {
        case "success":
            return "succeeded";
        case "failure":
            return "failed";
        default:
            return state;
    }
}

function parseDuration(duration: string): number {
    if (duration.length === 0) {
        return 0;
    }
    let seconds = 0;
    let multiple = 0;
    for (let i = duration.length; i >= 0; i--) {
        const ch = duration[i];
        if (ch === 's') {
            multiple = 1;
        } else if (ch === 'm') {
            multiple = 60;
        } else if (ch === 'h') {
            multiple = 60 * 60;
        } else if (ch >= '0' && ch <= '9') {
            seconds += Number(ch) * multiple;
            multiple *= 10;
        }
    }
    return seconds;
}

function formatDuration(seconds: number): string {
    const parts: string[] = [];
    if (seconds >= 3600) {
        const hours = Math.floor(seconds / 3600);
        parts.push(String(hours));
        parts.push('h');
        seconds = seconds % 3600;
    }
    if (seconds >= 60) {
        const minutes = Math.floor(seconds / 60);
        if (minutes > 0) {
            parts.push(String(minutes));
            parts.push('m');
            seconds = seconds % 60;
        }
    }
    if (seconds > 0) {
        parts.push(String(seconds));
        parts.push('s');
    }
    return parts.join('');
}

function drawJobHistogram(total: number, jobHistogram: JobHistogram, start: number, end: number, maximum: number): void {
    const startEl = document.getElementById("job-histogram-start") as HTMLSpanElement;
    if (startEl != null) {
        startEl.textContent = `${formatDuration(end - start)} ago`;
    }

    // make sure the empty table is hidden
    const tableEl = document.getElementById("job-histogram") as HTMLTableElement;
    const labelsEl = document.getElementById("job-histogram-labels") as HTMLDivElement;
    if (jobHistogram.length === 0) {
        tableEl.style.display = "none";
        labelsEl.style.display = "none";
        return;
    }
    tableEl.style.display = "";
    labelsEl.style.display = "";

    const el = document.getElementById("job-histogram-content") as HTMLTableSectionElement;
    el.title = `Showing ${jobHistogram.length} builds from last ${formatDuration(end - start)} by start time and duration, newest to oldest.`;
    const rows = 10;
    const width = 12;
    const cols = Math.round(el.clientWidth / width);

    // initialize the table if the row count changes
    if (el.childNodes.length !== rows) {
        el.innerHTML = "";
        for (let i = 0; i < rows; i++) {
            const tr = document.createElement('tr');
            for (let j = 0; j < cols; j++) {
                const td = document.createElement('td');
                tr.appendChild(td);
            }
            el.appendChild(tr);
        }
    }

    const buckets = jobHistogram.buckets(start, end, cols);
    buckets.limitMaximum(maximum);

    // show the max and mid y-axis labels rounded up to the nearest 10 minute mark
    let maxY = buckets.max;
    maxY = Math.ceil(maxY / 600);
    const yMax = document.getElementById("job-histogram-labels-y-max") as HTMLSpanElement;
    yMax.innerText = `${formatDuration(maxY * 600)}+`;
    const yMid = document.getElementById("job-histogram-labels-y-mid") as HTMLSpanElement;
    yMid.innerText = `${formatDuration(maxY / 2 * 600)}`;

    // populate the buckets
    buckets.data.forEach((bucket, colIndex) => {
        let lastRowIndex = 0;
        buckets.linearChunks(bucket, rows).forEach((samples, rowIndex) =>  {
            lastRowIndex = rowIndex + 1;
            const td = el.childNodes[rows - 1 - rowIndex].childNodes[cols - colIndex - 1] as HTMLTableCellElement;
            if (samples.length === 0) {
                td.removeAttribute('title');
                td.className = '';
                return;
            }
            td.dataset.sampleRow = String(samples[0].row);
            const failures = samples.reduce((sum, sample) => {
                return sample.state !== 'success' ? sum + 1 : sum;
            }, 0);
            if (failures === 0) {
                td.title = `${samples.length} succeeded`;
            } else {
                if (failures === samples.length) {
                    td.title = `${failures} failed`;
                } else {
                    td.title = `${failures}/${samples.length} failed`;
                }
            }
            td.style.opacity = String(0.2 + samples.length / bucket.length * 0.8);
            if (samples[0].row !== -1) {
                td.className = `active success-${Math.floor(10 - (failures / samples.length) * 10)}`;
            } else {
                td.className = `success-${Math.floor(10 - (failures / samples.length) * 10)}`;
            }
        });
        for (let rowIndex = lastRowIndex; rowIndex < rows; rowIndex++) {
            const td = el.childNodes[rows - 1 - rowIndex].childNodes[cols - colIndex - 1] as HTMLTableCellElement;
            td.removeAttribute('title');
            td.className = '';
        }
    });
}

function createSpyglassCell(url: string): HTMLTableDataCellElement {
    const i = icon.create('visibility', 'View in Spyglass');
    i.href = url;
    const c = document.createElement('td');
    c.classList.add('icon-cell');
    c.appendChild(i);
    return c;
}
