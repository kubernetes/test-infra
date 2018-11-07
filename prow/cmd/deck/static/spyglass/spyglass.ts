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
  document.getElementById(name + "-view")!.innerHTML = content;
}

// Show loading wheel over view
function insertLoading(name: string): void {
  document.getElementById(name + "-loading")!.style.display = "block";
}

window.onload = loadViews;

// Show all lines in specified log
function showAllLines(logID: string): void {
  document.getElementById(logID + "-show-all")!.style.display = "none";
  const log = document.getElementById(logID)!;
  const skipped = log.querySelectorAll<HTMLElement>(".skipped");
  for (let i = 0; i < skipped.length; i++) {
    skipped[i].classList.remove("skipped");
  }
  // hide any remaining "show hidden lines" buttons
  const showSkipped = log.querySelectorAll<HTMLElement>(".show-skipped");
  for (let i = 0; i < showSkipped.length; i++) {
    showSkipped[i].style.display = "none";
  }
}

function showLines(logID:string, linesID: string, skipID: string): void {
  // show a single group of hidden lines
  document.getElementById(linesID)!.classList.remove("skipped");
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

// The following form the "public API" and so are exposed until we do something
// better.

// Actual public API:
(window as any).refreshView = refreshView;

// Just for the build log:
(window as any).showAllLines = showAllLines;
(window as any).showLines = showLines;
(window as any).toggleExpansion = toggleExpansion;
