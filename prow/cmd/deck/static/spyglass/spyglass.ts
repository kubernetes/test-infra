interface Lens {
  Name: string;
  Title: string;
  HTMLView: string;
  Priority: number;
}

declare const src: string;
declare const viewerCache: {[key: string]: string[]};
declare const views: Lens[];

// Loads views for this job
function loadViews(): void {
  for (let view of views) {
    refreshView(view.Name, '{}');
  }
}

// refreshView refreshes a single view. Use this function to communicate with your viewer backend.
function refreshView(viewName: string, viewData: string): void {
  requestReload(viewName, createBody(viewName, viewData))
}

// Add a function here with a standard body response (list of matching artifacts, etc)
function createBody(viewName: string, viewData: string): string {
  return JSON.stringify({
    name: viewName,
    viewMatches: viewerCache[viewName],
    viewData: viewData
  });
}

// asynchronously requests a reloaded view of the provided viewer given a body request
function requestReload(name: string, body: string): void {
  insertLoading(name);
  const url = "/view/render?src="+encodeURIComponent(src)+"&name="+encodeURIComponent(name);
  const req = new XMLHttpRequest();
  req.open('POST', url, true);
  req.setRequestHeader('Content-Type', 'application/json');
  req.onreadystatechange = function() {
    if (req.readyState === 4 && req.status === 200) {
      const lensJson = JSON.parse(req.responseText) as Lens;
      insertView(name, lensJson.HTMLView);
    } else if (req.readyState === 4 && !(req.status === 200)) {
      insertView(name, "<div>Error: " + req.status +"</div>");
    }
  };
  req.send(body);
}

// Insert rendered view into page and remove loading wheel
function insertView(name: string, content: string): void {
  document.getElementById(name + "-loading")!.style.display = "none";

  const view = document.getElementById(name + "-view")!;
  view.innerHTML = content;
  // This is an icky workaround until we have viewer-specific scripts
  // https://github.com/kubernetes/test-infra/issues/8967
  if (name === "build-log-viewer") {
    const shown = view.getElementsByClassName("shown");
    for (let i = 0; i < shown.length; i++) {
      shown[i].innerHTML = ansiToHTML(shown[i].innerHTML);
    }
  }
}

// Show loading wheel over view
function insertLoading(name: string): void {
  document.getElementById(name + "-loading")!.style.display = "block";
}

window.onload = loadViews;

function showElem(elem: HTMLElement): void {
  elem.className = "shown";
  elem.innerHTML = ansiToHTML(elem.innerHTML);
}

// Show all lines in specified log
function showAllLines(logID: string): void {
  document.getElementById(logID + "-show-all")!.style.display = "none";
  const log = document.getElementById(logID)!;
  const skipped = log.querySelectorAll<HTMLElement>(".skipped");
  for (let i = 0; i < skipped.length; i++) {
    showElem(skipped[i]);
  }
  // hide any remaining "show hidden lines" buttons
  const showSkipped = log.querySelectorAll<HTMLElement>(".show-skipped");
  for (let i = 0; i < showSkipped.length; i++) {
    showSkipped[i].style.display = "none";
  }
}

function showLines(logID:string, linesID: string, skipID: string): void {
  showElem(document.getElementById(linesID)!);
  // hide the corresponding button
  document.getElementById(skipID)!.style.display = "none";
  // hide the "show all" button if nothing's left to show
  const log = document.getElementById(logID)!;
  const skipped = log.querySelectorAll<HTMLElement>(".skipped");
  if (skipped.length === 0) {
    document.getElementById(logID + "-show-all")!.style.display = "none";
  }
}

function toggleExpansion(bodyId: string, expanderId: string): void {
  const body = document.getElementById(bodyId)!;
  body.classList.toggle('hidden-tests');
  if (body.classList.contains('hidden-tests')) {
    document.getElementById(expanderId)!.innerHTML = 'expand_more';
  } else {
    document.getElementById(expanderId)!.innerHTML = 'expand_less';
  }
}

// given a string containing ansi formatting directives, return a new one
// with designated regions of text marked with the appropriate color directives,
// and with all unknown directives stripped
function ansiToHTML(orig: string): string {
  // Given a cmd (like "32" or "0;97"), some enclosed body text, and the original string,
  // either return the body wrapped in an element to achieve the desired result, or the
  // original string if nothing works.
  function annotate(cmd: string, body: string, orig: string): string {
    const code = +(cmd.replace('0;', ''));
    if (code === 0) {
      // reset
      return body;
    } else if (code === 1) {
      // bold
      return '<em>' + body + '</em>';
    } else if (30 <= code && code <= 37) {
      // foreground color
      return '<span class="ansi-' + (code - 30) + '">' + body + '</span>';
    } else if (90 <= code && code <= 97) {
      // foreground color, bright
      return '<span class="ansi-' + (code - 90 + 8) + '">' + body + '</span>';
    }
    return body;  // fallback: don't change anything
  }
  // Find commands, optionally followed by a bold command, with some content, then a reset command.
  // Unpaired commands are *not* handled here, but they're very uncommon.
  const filtered = orig.replace(/\033\[([0-9;]*)\w(\033\[1m)?([^\033]*?)\033\[0m/g, (match: string, cmd: string, bold: string, body: string, offset: number, str: string) => {
    if (bold !== undefined) {
      // normal code + bold
      return '<em>' + annotate(cmd, body, str) + '</em>';
    }
    return annotate(cmd, body, str);
  });
  // Strip out anything left over.
  return filtered.replace(/\033\[([0-9;]*\w)/g, (match: string, cmd: string, offset: number, str: string) => {
    console.log('unhandled ansi code: ', cmd, "context:", filtered);
    return '';
  });
}

// The following form the "public API" and so are exposed until we do something
// better.

// Actual public API:
(window as any).refreshView = refreshView;

// Just for the build log:
(window as any).showAllLines = showAllLines;
(window as any).showLines = showLines;

// Just for Junit test output
(window as any).toggleExpansion = toggleExpansion;
