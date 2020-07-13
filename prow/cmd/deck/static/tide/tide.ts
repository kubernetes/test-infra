import {PullRequest, TideData, TidePool} from '../api/tide';
import {tidehistory, tooltip} from '../common/common';

declare const tideData: TideData;

window.onload = (): void => {
    const infoDiv = document.getElementById("info-div")!;
    const infoH4 = infoDiv.getElementsByTagName("h4")[0]!;
    infoH4.addEventListener("click", infoToggle(infoDiv.getElementsByTagName("span")[0]), true);

    redraw();
};

function infoToggle(toToggle: HTMLElement): (event: Event) => void {
    return (event): void => {
        if (toToggle.className === "hidden") {
            toToggle.className = "";
            (event.target as HTMLElement).textContent = "Merge Requirements: (click to collapse)";
        } else {
            toToggle.className = "hidden";
            (event.target as HTMLElement).textContent = "Merge Requirements: (click to expand)";
        }
    };
}

function redraw(): void {
    redrawQueries();
    redrawPools();
}

function createLink(href: string, text: string): HTMLAnchorElement {
    const a = document.createElement("a");
    a.href = href;
    a.appendChild(document.createTextNode(text));
    return a;
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
  el.classList.add("mdl-shadow--2dp", "label", escapedName);
  el.textContent = label;

  return el;
}

function createStrong(text: string): HTMLElement {
    const s = document.createElement("strong");
    s.appendChild(document.createTextNode(text));
    return s;
}

function fillDetail(data: string | string[] | undefined, type: string, connector: string, container: HTMLElement, styleData: (content: string) => Node) {
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

function redrawQueries(): void {
    const queries = document.getElementById("queries")!;
    while (queries.firstChild) {
        queries.removeChild(queries.firstChild);
    }

    if (!tideData.Queries) {
        return;
    }
    for (let i = 0; i < tideData.Queries.length; i++) {
        const query = tideData.Queries[i];
        const tideQuery = tideData.TideQueries[i];

        // create list entry for the query, all details will be within this element
        const li = document.createElement("li");

        // GitHub query search link
        const a = createLink(
            `/github-link?dest=search?utf8=${encodeURIComponent("\u2713")}&q=${encodeURIComponent(query)}`,
            "GitHub Search Link",
        );
        li.appendChild(a);
        li.appendChild(document.createTextNode(" - Meaning: Is an open Pull Request"));

        // build the description
        // all queries should implicitly mean this
        // add the list of repos, defaulting to an empty array if no repos have been provided.
        const orgs = tideQuery.orgs || [];
        const repos = tideQuery.repos || [];
        const excludedRepos = tideQuery.excludedRepos || [];
        if (orgs.length > 0) {
            li.appendChild(document.createTextNode(" in one of the following orgs: "));
            const ul = document.createElement("ul");
            const innerLi = document.createElement("li");
            for (let j = 0; j < orgs.length; j++) {
                innerLi.appendChild(createLink("/github-link?dest=" + orgs[j], orgs[j]));
                if (j + 1 < repos.length) {
                    innerLi.appendChild(document.createTextNode(", "));
                }
            }
            ul.appendChild(innerLi);
            li.appendChild(ul);
        }
        if (repos.length > 0) {
            let reposText = " in one of the following repos: ";
            if (orgs.length > 0) {
                reposText = " or " + reposText;
            }
            li.appendChild(document.createTextNode(reposText));
            const ul = document.createElement("ul");
            const innerLi = document.createElement("li");
            for (let j = 0; j < repos.length; j++) {
                innerLi.appendChild(createLink("/github-link?dest=" + repos[j], repos[j]));
                if (j + 1 < repos.length) {
                    innerLi.appendChild(document.createTextNode(", "));
                }
            }
            ul.appendChild(innerLi);
            li.appendChild(ul);
        }
        if (excludedRepos.length > 0) {
            li.appendChild(document.createTextNode(" but NOT in any of the following excluded repos: "));
            const ul = document.createElement("ul");
            const innerLi = document.createElement("li");
            for (let j = 0; j < excludedRepos.length; j++) {
                innerLi.appendChild(createLink("/github-link?dest=" + excludedRepos[j], excludedRepos[j]));
                if (j + 1 < excludedRepos.length) {
                    innerLi.appendChild(document.createTextNode(", "));
                }
            }
            ul.appendChild(innerLi);
            li.appendChild(ul);
        }
        // required labels
        fillDetail(tideQuery.labels, "labels", "with ", li, (data) => createLabelEl(data));
        // required to be not present labels
        fillDetail(tideQuery.missingLabels, "labels", "without ", li, (data) => createLabelEl(data));
        // list milestone if existed
        fillDetail(tideQuery.milestone, "milestone", "with ", li, (data) => document.createTextNode(data));
        // list all excluded branches
        fillDetail(tideQuery.excludedBranches, "branches", "exclude ", li, (data) => document.createTextNode(data));
        // list all included branches
        fillDetail(tideQuery.includedBranches, "branches", "targeting ", li, (data) => document.createTextNode(data));
        // GitHub native review required
        const reviewApprovedRequired = tideQuery.hasOwnProperty("reviewApprovedRequired") && tideQuery.reviewApprovedRequired;
        if (reviewApprovedRequired) {
            li.appendChild(document.createTextNode("and must be "));
            li.appendChild(createLink(
                "https://help.github.com/articles/about-pull-request-reviews/",
                "approved by GitHub review",
            ));
        }

        // actually add the entry
        queries.appendChild(li);
    }
}

function redrawPools(): void {
    const pools = document.getElementById("pools")!.getElementsByTagName("tbody")[0];
    while (pools.firstChild) {
        pools.removeChild(pools.firstChild);
    }

    if (!tideData.Pools) {
        return;
    }
    for (const pool of tideData.Pools) {
        const r = document.createElement("tr");

        r.appendChild(createHistoryCell(pool));
        r.appendChild(createRepoCell(pool));
        r.appendChild(createActionCell(pool));
        r.appendChild(createBatchCell(pool));
        r.appendChild(createPRCell(pool, pool.SuccessPRs));
        r.appendChild(createPRCell(pool, pool.PendingPRs));
        r.appendChild(createPRCell(pool, pool.MissingPRs));

        pools.appendChild(r);
    }
}

function createHistoryCell(pool: TidePool): HTMLTableDataCellElement {
    const td = document.createElement("td");
    td.classList.add("icon-cell");
    td.appendChild(tidehistory.poolIcon(pool.Org, pool.Repo, pool.Branch));
    return td;
}

function createRepoCell(pool: TidePool): HTMLTableDataCellElement {
    const deckLink = `/?repo=` + encodeURIComponent(`${pool.Org}/${pool.Repo}`);
    const branchLink = `/github-link?dest=${pool.Org}/${pool.Repo}/tree/${pool.Branch}`;
    const linksTD = document.createElement("td");
    linksTD.appendChild(createLink(deckLink, `${pool.Org}/${pool.Repo}`));
    linksTD.appendChild(document.createTextNode(" "));
    linksTD.appendChild(createLink(branchLink, pool.Branch));
    return linksTD;
}

function createActionCell(pool: TidePool): HTMLTableDataCellElement {
    const targeted = pool.Target && pool.Target.length;
    const blocked = pool.Blockers && pool.Blockers.length;
    let action = pool.Action.replace("_", " ");
    if (targeted || blocked) {
        action += ": ";
    }
    const c = document.createElement("td");
    c.appendChild(document.createTextNode(action));

    if (blocked) {
        c.classList.add("blocked");
        addBlockersToElem(c, pool);
    } else if (targeted) {
        addPRsToElem(c, pool, pool.Target);
    }
    return c;
}

function createPRCell(pool: TidePool, prs: PullRequest[]): HTMLTableDataCellElement {
    const c = document.createElement("td");
    addPRsToElem(c, pool, prs);
    return c;
}

function createBatchCell(pool: TidePool): HTMLTableDataCellElement {
    const td = document.createElement('td');
    if (pool.BatchPending) {
        const numbers = pool.BatchPending.map((p) => String(p.Number));
        const batchRef = `${pool.Branch},${numbers.join(',')}`;
        const encodedRepo = encodeURIComponent(`${pool.Org}/${pool.Repo}`);
        const href = `/?repo=${encodedRepo}&type=batch&pull=${encodeURIComponent(batchRef)}`;
        const link = document.createElement('a');
        link.href = href;
        for (let i = 0; i < pool.BatchPending.length; i++) {
            const pr = pool.BatchPending[i];
            const text = document.createElement('span');
            text.appendChild(document.createTextNode("#" + String(pr.Number)));
            text.id = `pr-${pool.Org}-${pool.Repo}-${pr.Number}-${nextID()}`;
            if (pr.Title) {
                const tip = tooltip.forElem(text.id, document.createTextNode(pr.Title));
                text.appendChild(tip);
            }
            link.appendChild(text);
            // Add a space after each PR number except the last.
            if (i + 1 < pool.BatchPending.length) {
                link.appendChild(document.createTextNode(" "));
            }
        }
        td.appendChild(link);
    }
    return td;
}

// addPRsToElem adds a space separated list of PR numbers that link to the corresponding PR on github.
function addPRsToElem(elem: HTMLElement, pool: TidePool, prs?: PullRequest[]): void {
    if (prs) {
        for (let i = 0; i < prs.length; i++) {
            const a = document.createElement("a");
            a.href = `/github-link?dest=${pool.Org}/${pool.Repo}/pull/${prs[i].Number}`;
            a.appendChild(document.createTextNode("#" + prs[i].Number));
            a.id = `pr-${pool.Org}-${pool.Repo}-${prs[i].Number}-${nextID()}`;
            if (prs[i].Title) {
                const tip = tooltip.forElem(a.id, document.createTextNode(prs[i].Title));
                a.appendChild(tip);
            }
            elem.appendChild(a);
            // Add a space after each PR number except the last.
            if (i + 1 < prs.length) {
                elem.appendChild(document.createTextNode(" "));
            }
        }
    }
}

// addBlockersToElem adds a space separated list of Issue numbers that link to the
// corresponding Issues on github that are blocking merge.
function addBlockersToElem(elem: HTMLElement, pool: TidePool): void {
    if (!pool.Blockers) {
        return;
    }
    for (let i = 0; i < pool.Blockers.length; i++) {
        const b = pool.Blockers[i];
        const a = document.createElement("a");
        a.href = b.URL;
        a.appendChild(document.createTextNode("#" + b.Number));
        a.id = `blocker-${pool.Org}-${pool.Repo}-${b.Number}-${nextID()}`;
        a.appendChild(tooltip.forElem(a.id, document.createTextNode(b.Title)));

        elem.appendChild(a);
        // Add a space after each PR number except the last.
        if (i + 1 < pool.Blockers.length) {
            elem.appendChild(document.createTextNode(" "));
        }
    }
}

let idCounter = 0;
function nextID(): string {
    idCounter++;
    return "elemID-" + String(idCounter);
}
