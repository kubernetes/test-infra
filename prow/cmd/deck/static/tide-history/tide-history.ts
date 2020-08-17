import moment from "moment";
import {ProwJobState} from "../api/prow";
import {HistoryData, Record} from "../api/tide-history";
import {cell} from "../common/common";
import {getParameterByName} from "../common/urls";

declare const tideHistory: HistoryData;

const recordDisplayLimit = 500;

interface FilteredRecord extends Record {
  // The following are not initially present and are instead populated based on the 'History' map key while filtering.
  repo: string;
  branch: string;
}

interface Options {
  repos: {[key: string]: boolean};
  branchs: {[key: string]: boolean};  // This is intentionally a typo to make pluralization easy.
  actions: {[key: string]: boolean};
  states: {[key: string]: boolean};
  authors: {[key: string]: boolean};
  pulls: {[key: string]: boolean};
}

function optionsForRepoBranch(repo: string, branch: string): Options {
  const opts: Options = {
    actions: {},
    authors: {},
    branchs: {},
    pulls: {},
    repos: {},
    states: {},
  };

  const hist: {[key: string]: Record[]} = typeof tideHistory !== 'undefined' ? tideHistory.History : {};
  const poolKeys = Object.keys(hist);
  for (const poolKey of poolKeys) {
    const match = RegExp('(.*?):(.*)').exec(poolKey);
    if (!match) {
      continue;
    }
    const recRepo = match[1];
    const recBranch = match[2];

    opts.repos[recRepo] = true;
    if (!repo || repo === recRepo) {
      opts.branchs[recBranch] = true;
      if (!branch || branch === recBranch) {
        const recs = hist[poolKey];
        for (const rec of recs) {
          opts.actions[rec.action] = true;
          opts.states[errorState(rec.err)] = true;
          for (const pr of rec.target || []) {
            opts.authors[pr.author] = true;
            opts.pulls[pr.number] = true;
          }
        }
      }
    }
  }

  return opts;
}

function errorState(err?: string): ProwJobState {
  return err ? "failure" : "success";
}

function redrawOptions(opts: Options) {
  const repos = Object.keys(opts.repos).sort();
  addOptions(repos, "repo");
  const branchs = Object.keys(opts.branchs).sort(); // English sucks.
  addOptions(branchs, "branch");
  const actions = Object.keys(opts.actions).sort();
  addOptions(actions, "action");
  const authors = Object.keys(opts.authors).sort(
    (a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
  addOptions(authors, "author");
  const pulls = Object.keys(opts.pulls).sort((a, b) => Number(a) - Number(b));
  addOptions(pulls, "pull");
  const states = Object.keys(opts.states).sort();
  addOptions(states, "state");
}

window.onload = (): void => {
  const topNavigator = document.getElementById("top-navigator")!;
  let navigatorTimeOut: any;
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

  // Register selection on change functions
  const filterBox = document.getElementById("filter-box")!;
  const options = filterBox.querySelectorAll("select")!;
  options.forEach((opt) => {
      opt.onchange = () => {
          redraw();
      };
  });

  // set dropdown based on options from query string
  redrawOptions(optionsForRepoBranch("", ""));
  redraw();
};

function addOptions(options: string[], selectID: string): string | undefined {
  const sel = document.getElementById(selectID)! as HTMLSelectElement;
  while (sel.length > 1) {
    sel.removeChild(sel.lastChild!);
  }
  const param = getParameterByName(selectID);
  for (const option of options) {
    const o = document.createElement("option");
    o.value = option;
    o.text = o.value;
    if (param && option === param) {
      o.selected = true;
    }
    sel.appendChild(o);
  }
  return param;
}

function equalSelected(sel: string, t: string): boolean {
  return sel === "" || sel === t;
}

function redraw(): void {
  const args: string[] = [];

  function getSelection(name: string): string {
    const sel = (document.getElementById(name) as HTMLSelectElement).value;
    if (sel && opts && !opts[name + 's' as keyof Options][sel]) {
      return "";
    }
    if (sel !== "") {
      args.push(`${name}=${encodeURIComponent(sel)}`);
    }
    return sel;
  }

  const initialRepoSel = (document.getElementById("repo") as HTMLSelectElement).value;
  const initialBranchSel = (document.getElementById("branch") as HTMLSelectElement).value;

  const opts = optionsForRepoBranch(initialRepoSel, initialBranchSel);
  const repoSel = getSelection("repo");
  const branchSel = getSelection("branch");
  const pullSel = getSelection("pull");
  const authorSel = getSelection("author");
  const actionSel = getSelection("action");
  const stateSel = getSelection("state");

  if (window.history && window.history.replaceState !== undefined) {
    if (args.length > 0) {
      history.replaceState(null, "", "/tide-history?" + args.join('&'));
    } else {
      history.replaceState(null, "", "/tide-history");
    }
  }
  redrawOptions(opts);

  let filteredRecs: FilteredRecord[] = [];
  const hist: {[key: string]: Record[]} = typeof tideHistory !== 'undefined' ? tideHistory.History : {};
  const poolKeys = Object.keys(hist);
  for (const poolKey of poolKeys) {
    const match = RegExp('(.*?):(.*)').exec(poolKey);
    if (!match || match.length !== 3) {
      return;
    }
    const repo = match[1];
    const branch = match[2];

    if (!equalSelected(repoSel, repo)) {
      continue;
    }
    if (!equalSelected(branchSel, branch)) {
      continue;
    }

    const recs = hist[poolKey];
    for (const rec of recs) {
      if (!equalSelected(actionSel, rec.action)) {
        continue;
      }
      if (!equalSelected(stateSel, errorState(rec.err))) {
        continue;
      }

      let anyTargetMatches = false;
      for (const pr of rec.target || []) {
        if (!equalSelected(pullSel, pr.number.toString())) {
          continue;
        }
        if (!equalSelected(authorSel, pr.author)) {
          continue;
        }

        anyTargetMatches = true;
        break;
      }
      if (!anyTargetMatches) {
        continue;
      }

      const filtered = rec as FilteredRecord;
      filtered.repo = repo;
      filtered.branch = branch;
      filteredRecs.push(filtered);
    }
  }
  // Sort by descending time.
  filteredRecs = filteredRecs.sort((a, b) => a.time > b.time ? -1 : (a.time < b.time ? 1 : 0));
  redrawRecords(filteredRecs);
}

function redrawRecords(recs: FilteredRecord[]): void {
  const records = document.getElementById("records")!.getElementsByTagName(
    "tbody")[0];
  while (records.firstChild) {
    records.removeChild(records.firstChild);
  }

  let lastKey = '';
  const displayCount = Math.min(recs.length, recordDisplayLimit);
  for (let i = 0; i < displayCount; i++) {
    const rec = recs[i];
    const r = document.createElement("tr");

    r.appendChild(cell.state(errorState(rec.err)));
    const key = `${rec.repo} ${rec.branch} ${rec.baseSHA || ""}`;
    if (key !== lastKey) {
      // This is a different pool or base branch commit than the previous row.
      lastKey = key;
      r.className = "changed";

      r.appendChild(cell.link(
        `${rec.repo} ${rec.branch}`,
        `/github-link?dest=${rec.repo}/tree/${rec.branch}`,
      ));
      if (rec.baseSHA) {
          r.appendChild(cell.link(
            rec.baseSHA.slice(0, 7),
            `/github-link?dest=${rec.repo}/commit/${rec.baseSHA}`,
          ));
      } else {
          r.appendChild(cell.text(""));
      }
    } else {
      // Don't render identical cells for the same pool+baseSHA
      r.appendChild(cell.text(""));
      r.appendChild(cell.text(""));
    }
    r.appendChild(cell.text(rec.action));
    r.appendChild(targetCell(rec));
    r.appendChild(cell.time(nextID(), moment(rec.time)));
    r.appendChild(cell.text(rec.err || ""));
    records.appendChild(r);
  }
  const recCount = document.getElementById("record-count")!;
  recCount.textContent = `Showing ${displayCount}/${recs.length} records`;
}

function targetCell(rec: FilteredRecord): HTMLTableDataCellElement {
  const target = rec.target || [];
  switch (target.length) {
    case 0:
      return cell.text("");
    case 1: {
      const pr = target[0];
      return cell.prRevision(rec.repo, pr);
    }
    default: {
      // Multiple PRs in 'target'. Add them all to the cell, but on separate lines.
      const td = document.createElement("td");
      td.style.whiteSpace = "pre";
      for (const pr of target) {
        cell.addPRRevision(td, rec.repo, pr);
        td.appendChild(document.createTextNode("\n"));
      }
      return td;
    }
  }
}

let idCounter = 0;
function nextID(): string {
  idCounter++;
  return "histID-" + String(idCounter);
}
