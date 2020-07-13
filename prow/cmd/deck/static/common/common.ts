import moment from "moment";
import {ProwJobState, Pull} from "../api/prow";

// This file likes namespaces, so stick with it for now.
/* tslint:disable:no-namespace */

// The cell namespace exposes functions for constructing common table cells.
export namespace cell {

  export function text(content: string): HTMLTableDataCellElement {
    const c = document.createElement("td");
    c.appendChild(document.createTextNode(content));
    return c;
  }

  export function time(id: string, when: moment.Moment): HTMLTableDataCellElement {
    const tid = "time-cell-" + id;
    const main = document.createElement("div");
    const isADayOld = when.isBefore(moment().startOf('day'));
    main.textContent = when.format(isADayOld ? 'MMM DD HH:mm:ss' : 'HH:mm:ss');
    main.id = tid;

    const tip = document.createElement("div");
    tip.textContent = when.format('MMM DD YYYY, HH:mm:ss [UTC]ZZ');
    tip.setAttribute("data-mdl-for", tid);
    tip.classList.add("mdl-tooltip", "mdl-tooltip--large");

    const c = document.createElement("td");
    c.appendChild(main);
    c.appendChild(tip);

    return c;
  }

  export function link(displayText: string, url: string): HTMLTableDataCellElement {
    const c = document.createElement("td");
    const a = document.createElement("a");
    a.href = url;
    a.appendChild(document.createTextNode(displayText));
    c.appendChild(a);
    return c;
  }

  export function state(s: ProwJobState): HTMLTableDataCellElement {
    const c = document.createElement("td");
    if (!s) {
      c.appendChild(document.createTextNode(""));
      return c;
    }
    c.classList.add("icon-cell");

    let displayState = stateToAdj(s);
    displayState = displayState[0].toUpperCase() + displayState.slice(1);
    let displayIcon = "";
    switch (s) {
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
    const stateIndicator = document.createElement("i");
    stateIndicator.classList.add("material-icons", "state", s);
    stateIndicator.innerText = displayIcon;
    c.appendChild(stateIndicator);
    c.title = displayState;

    return c;
  }

  function stateToAdj(s: ProwJobState): string {
    switch (s) {
      case "success":
        return "succeeded";
      case "failure":
        return "failed";
      default:
        return s;
    }
  }

  export function commitRevision(repo: string, ref: string, SHA: string, pushCommitLink: string): HTMLTableDataCellElement {
    const c = document.createElement("td");
    const bl = document.createElement("a");
    bl.href = pushCommitLink;
    if (!bl.href) {
      bl.href = `/github-link?dest=${repo}/commit/${SHA}`;
    }
    bl.text = `${ref} (${SHA.slice(0, 7)})`;
    c.appendChild(bl);
    return c;
  }

  export function prRevision(repo: string, pull: Pull): HTMLTableDataCellElement {
    const td = document.createElement("td");
    addPRRevision(td, repo, pull);
    return td;
  }

  let idCounter = 0;
  function nextID(): string {
    idCounter++;
    return "tipID-" + String(idCounter);
  }

  export function addPRRevision(elem: Node, repo: string, pull: Pull): void {
    elem.appendChild(document.createTextNode("#"));
    const pl = document.createElement("a");
    if (pull.link) {
      pl.href = pull.link;
    } else {
      pl.href = `/github-link?dest=${repo}/pull/${pull.number}`;
    }
    pl.text = pull.number.toString();
    if (pull.title) {
      pl.id = `pr-${repo}-${pull.number}-${nextID()}`;
      const tip = tooltip.forElem(pl.id, document.createTextNode(pull.title));
      pl.appendChild(tip);
    }
    elem.appendChild(pl);
    if (pull.sha) {
      elem.appendChild(document.createTextNode(" ("));
      const cl = document.createElement("a");
      if (pull.commit_link) {
        cl.href = pull.commit_link;
      } else {
        cl.href = `/github-link?dest=${repo}/pull/${pull.number}/commits/${pull.sha}`;
      }
      cl.text = pull.sha.slice(0, 7);
      elem.appendChild(cl);
      elem.appendChild(document.createTextNode(")"));
    }
    if (pull.author) {
      elem.appendChild(document.createTextNode(" by "));
      const al = document.createElement("a");
      if (pull.author_link) {
        al.href = pull.author_link;
      } else {
        al.href = "/github-link?dest=" + pull.author;
      }
      al.text = pull.author;
      elem.appendChild(al);
    }
  }
}

export namespace tooltip {
  export function forElem(elemID: string, tipElem: Node): Node {
    const tip = document.createElement("div");
    tip.appendChild(tipElem);
    tip.setAttribute("data-mdl-for", elemID);
    tip.classList.add("mdl-tooltip", "mdl-tooltip--large");
    tip.style.whiteSpace = "normal";
    return tip;
  }
}

export namespace icon {
  export function create(iconString: string, tip: string = "", onClick?: (this: HTMLElement, ev: MouseEvent) => any): HTMLAnchorElement {
    const i = document.createElement("i");
    i.classList.add("icon-button", "material-icons");
    i.innerHTML = iconString;
    if (tip !== "") {
       i.title = tip;
    }
    if (onClick) {
      i.addEventListener("click", onClick);
    }

    const container = document.createElement("a");
    container.appendChild(i);
    container.classList.add("mdl-button", "mdl-js-button", "mdl-button--icon");

    return container;
  }
}

export namespace tidehistory {
  export function poolIcon(org: string, repo: string, branch: string): HTMLAnchorElement {
    const link = icon.create("timeline", "Pool History");
    const encodedRepo = encodeURIComponent(`${org}/${repo}`);
    const encodedBranch = encodeURIComponent(branch);
    link.href = `/tide-history?repo=${encodedRepo}&branch=${encodedBranch}`;
    return link;
  }

  export function authorIcon(author: string): HTMLAnchorElement {
    const link = icon.create("timeline", "Personal Tide History");
    const encodedAuthor = encodeURIComponent(author);
    link.href = `/tide-history?author=${encodedAuthor}`;
    return link;
  }
}

export function getCookieByName(name: string): string {
  if (!document.cookie) {
    return "";
  }
  const docCookies = decodeURIComponent(document.cookie).split(";");
  for (const cookie of docCookies) {
    const c = cookie.trim();
    const pref = name + "=";
    if (c.indexOf(pref) === 0) {
      return c.slice(pref.length);
    }
  }
  return "";
}
