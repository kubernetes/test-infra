import moment from "moment";
import {ProwJobState, Pull} from "../api/prow";
import {relativeURL} from "./urls";

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

    const tip = tooltip.forElem(tid, document.createTextNode(when.format('MMM DD YYYY, HH:mm:ss [UTC]ZZ')));
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
  export function forElem(elemID: string, tipElem: Node): HTMLElement {
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

export function createRerunProwJobIcon(modal: HTMLElement, rerunElement: HTMLElement, prowjob: string, rerunCreatesJob: boolean, csrfToken: string): HTMLElement {
  const url = `${location.protocol}//${location.host}/rerun?prowjob=${prowjob}`;
  const i = icon.create("refresh", "Show instructions for rerunning this job");

  window.onkeydown = (event: any) => {
    if ( event.key === "Escape" ) {
      modal.style.display = "none";
    }
  };
  window.onclick = (event: any) => {
    if (event.target === modal) {
      modal.style.display = "none";
    }
  };

  // we actually want to know whether the "access-token-session" cookie exists, but we can't always
  // access it from the frontend. "github_login" should be set whenever "access-token-session" is
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
        runButton.innerHTML = "<button class='mdl-button mdl-js-button mdl-button--raised mdl-button--colored'>Rerun</button>";
        runButton.onclick = async () => {
            gtag("event", "rerun", {
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
            if (result.status === 401) {
                window.location.href = window.location.origin + `/github-login?dest=${relativeURL({rerun: "gh_redirect"})}`;
            } else {
                rerunElement.innerHTML = data;
            }
        };
        rerunElement.appendChild(runButton);
    }
  };

  return i;
}

function copyToClipboardWithToast(text: string): void {
  copyToClipboard(text);

  const toast = document.getElementById("toast") as SnackbarElement<HTMLDivElement>;
  toast.MaterialSnackbar.showSnackbar({message: "Copied to clipboard"});
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

export function formatDuration(seconds: number): string {
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
  if (seconds >= 0) {
      parts.push(String(seconds));
      parts.push('s');
  }
  return parts.join('');
}
