import moment from "moment";
import {Job, JobState, JobType} from "../api/prow";
import {cell} from "../common/common";
import {FuzzySearch} from './fuzzy-search';

declare const allBuilds: Job[];
declare const spyglass: boolean;

// http://stackoverflow.com/a/5158301/3694
function getParameterByName(name: string): string | null {
    const match = RegExp(`[?&]${name}=([^&/]*)`).exec(
        window.location.search);
    return match && decodeURIComponent(match[1].replace(/\+/g, ' '));
}

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

function addOptions(options: string[], selectID: string): string | null {
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

function redraw(fz: FuzzySearch): void {
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

    function getSelectionFuzzySearch(id: string, inputId: string): string {
        const input = document.getElementById(inputId) as HTMLInputElement;
        const inputText = input.value;
        if (inputText !== "" && opts && !opts[id + 's' as keyof RepoOptions][inputText]) {
            return "";
        }
        if (inputText !== "") {
            args.push(`${id}=${encodeURIComponent(inputText)}`);
        }

        return inputText;
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
    let totalJob = 0;
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
        if (!equalSelected(jobSel, build.job)) {
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
        totalJob ++;
        jobCountMap.set(build.state, jobCountMap.get(build.state)! + 1);
        if (totalJob > 499) {
            continue;
        }
        const r = document.createElement("tr");
        r.appendChild(cell.state(build.state));
        if (build.pod_name) {
            const icon = createIcon("description", "Build log");
            icon.href = `log?job=${build.job}&id=${build.build_id}`;
            const c = document.createElement("td");
            c.classList.add("icon-cell");
            c.appendChild(icon);
            r.appendChild(c);
        } else {
            r.appendChild(cell.text(""));
        }
        r.appendChild(createRerunCell(modal, rerunCommand, build.prow_job));
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
    const jobCount = document.getElementById("job-count")!;
    jobCount.textContent = `Showing ${Math.min(totalJob, 500)}/${totalJob} jobs`;
    drawJobBar(totalJob, jobCountMap);
}

function createRerunCell(modal: HTMLElement, rerunElement: HTMLElement, prowjob: string): HTMLTableDataCellElement {
    const url = `https://${window.location.hostname}/rerun?prowjob=${prowjob}`;
    const c = document.createElement("td");
    const icon = createIcon("refresh", "Show instructions for rerunning this job");
    icon.onclick = () => {
        modal.style.display = "block";
        rerunElement.innerHTML = `kubectl create -f "<a href="${url}">${url}</a>"`;
        const copyButton = document.createElement('a');
        copyButton.className = "mdl-button mdl-js-button mdl-button--icon";
        copyButton.onclick = () => copyToClipboardWithToast(`kubectl create -f "${url}"`);
        copyButton.innerHTML = "<i class='material-icons state triggered' style='color: gray'>file_copy</i>";
        rerunElement.appendChild(copyButton);
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
    for (let i = 1; i < build.refs.pulls.length; i++) {
        if (i !== 1) {
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

function createSpyglassCell(url: string): HTMLTableDataCellElement {
    const icon = createIcon('visibility', 'View in Spyglass');
    icon.href = url;
    const c = document.createElement('td');
    c.classList.add('icon-cell');
    c.appendChild(icon);
    return c;
}

function createIcon(iconString: string, tooltip: string = ""): HTMLAnchorElement {
    const icon = document.createElement("i");
    icon.classList.add("icon-button", "material-icons");
    icon.innerHTML = iconString;
    if (tooltip !== "") {
        icon.title = tooltip;
    }

    const container = document.createElement("a");
    container.appendChild(icon);
    container.classList.add("mdl-button", "mdl-js-button", "mdl-button--icon");

    return container;
}
