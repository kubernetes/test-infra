function showElem(elem: HTMLElement): void {
  elem.className = 'shown';
  elem.innerHTML = ansiToHTML(elem.innerHTML);
}

// given a string containing ansi formatting directives, return a new one
// with designated regions of text marked with the appropriate color directives,
// and with all unknown directives stripped
function ansiToHTML(orig: string): string {
  // Given a cmd (like "32" or "0;97"), some enclosed body text, and the original string,
  // either return the body wrapped in an element to achieve the desired result, or the
  // original string if nothing works.
  function annotate(cmd: string, body: string): string {
    const code = +(cmd.replace('0;', ''));
    if (code === 0) {
      // reset
      return body;
    } else if (code === 1) {
      // bold
      return `<strong>${body}</strong>`;
    } else if (code === 3) {
      // italic
      return `<em>${body}</em>`;
    } else if (30 <= code && code <= 37) {
      // foreground color
      return `<span class="ansi-${(code - 30)}">${body}</span>`;
    } else if (90 <= code && code <= 97) {
      // foreground color, bright
      return `<span class="ansi-${(code - 90 + 8)}">${body}</span>`;
    }
    return body;  // fallback: don't change anything
  }
  // Find commands, optionally followed by a bold command, with some content, then a reset command.
  // Unpaired commands are *not* handled here, but they're very uncommon.
  const filtered = orig.replace(/\033\[([0-9;]*)\w(\033\[1m)?([^\033]*?)\033\[0m/g, (match: string, cmd: string, bold: string, body: string, offset: number, str: string) => {
    if (bold !== undefined) {
      // normal code + bold
      return `<strong>${annotate(cmd, body)}</strong>`;
    }
    return annotate(cmd, body);
  });
  // Strip out anything left over.
  return filtered.replace(/\033\[([0-9;]*\w)/g, (match: string, cmd: string, offset: number, str: string) => {
    console.log('unhandled ansi code: ', cmd, "context:", filtered);
    return '';
  });
}

interface ArtifactRequest {
  artifact: string;
  bottom: number;
  length: number;
  offset: number;
  startLine: number;
  top: number;
}

async function replaceElementWithContent(element: HTMLDivElement, top: number, bottom: number) {

  // <div data-foo="1" data-bar="this"> will show up as element.dataset = {"foo": "1", "bar": "this"}
  const {artifact, offset, length, startLine} = element.dataset;

  // length! => we know these values are non-null:
  // - we know this because its tightly coupled with template.html
  // TODO(fejta): consider more robust code, looser coupling.
  const r: ArtifactRequest = {
    artifact: artifact!,
    bottom,
    length: Number(length!),
    offset: Number(offset!),
    startLine: Number(startLine!),
    top,
  };
  const content = await spyglass.request(JSON.stringify(r));
  showElem(element);
  element.outerHTML = ansiToHTML(content);
  fixLinks(document.documentElement);
  for (const button of Array.from(document.querySelectorAll<HTMLDivElement>(".show-skipped"))) {
    if (button.classList.contains("showable")) {
      continue;
    }
    button.addEventListener('click', handleShowSkipped);
    button.classList.add("showable");
  }

  for (const button of Array.from(document.querySelectorAll<HTMLDivElement>(".show-skipped"))) {
    button.addEventListener('click', handleShowSkipped);
  }

  // Remove the "show all" button if we no longer need it.
  // TODO(fejta): avoid id selectors: https://google.github.io/styleguide/htmlcssguide.html#ID_Selectors
  const log = document.getElementById(`${r.artifact}-content`)!;
  const skipped = log.querySelectorAll<HTMLElement>(".show-skipped");
  if (skipped.length === 0) {
    const button = document.querySelector('button.show-all-button')!;
    button.parentNode!.removeChild(button);
  }
  spyglass.contentUpdated();
}

async function handleShowSkipped(this: HTMLDivElement, e: MouseEvent): Promise<void> {
  // Don't do anything unless they actually clicked the button.
  let target: HTMLButtonElement;
  if (!(e.target instanceof HTMLButtonElement)) {
    if (e.target instanceof Node && e.target.parentElement instanceof HTMLButtonElement) {
      target = e.target.parentElement;
    } else {
      return;
    }
  } else {
    target = e.target;
  }

  const classes: DOMTokenList = target.classList;
  let top = 0;
  let bottom = 0;
  if (classes.contains("top")) {
    top = 10;
  }
  if (classes.contains("bottom")) {
    bottom = 10;
  }
  await replaceElementWithContent(this, top, bottom);
}

async function handleShowAll(this: HTMLButtonElement) {
  // Remove ourselves immediately.
  if (this.parentElement) {
    this.parentElement.removeChild(this);
  }

  const {artifact} = this.dataset;
  const content = await spyglass.request(JSON.stringify({artifact, offset: 0, length: -1}));
  document.getElementById(`${artifact}-content`)!.innerHTML = `<tbody class="shown">${ansiToHTML(content)}</tbody>`;
  spyglass.contentUpdated();
}

function handleLineLink(e: MouseEvent): void {
  if (!e.target) {
    return;
  }
  const el = e.target as HTMLElement;
  if (!el.dataset.lineNumber) {
    return;
  }
  const multiple = e.shiftKey;
  const goal = Number(el.dataset.lineNumber);
  if (isNaN(goal)) {
    return;
  }
  let result = parseHash();
  if (result === null || !multiple) {
    result = ["", goal, goal];
  }
  let startNum = result[1];
  let endNum = result[2];
  if (goal > startNum) {
    endNum = goal;
  } else {
    [startNum, endNum] = [goal, startNum];
  }
  if (endNum !== startNum) {
    location.hash = `#${el.dataset.artifact}:${startNum}-${endNum}`;
  } else {
    location.hash = `#${el.dataset.artifact}:${startNum}`;
  }
  e.preventDefault();
}

function clearHighlightedLines(): void {
  for (const oldEl of Array.from(document.querySelectorAll('.highlighted-line'))) {
    oldEl.classList.remove('highlighted-line');
  }
}

function highlightLine(element: HTMLElement): void {
  element.classList.add('highlighted-line');
}

function fixLinks(parent: HTMLElement): void {
  const links = parent.querySelectorAll<HTMLAnchorElement>('a[data-artifact][data-line-number]');
  for (const link of Array.from(links)) {
    link.href = spyglass.makeFragmentLink(`${link.dataset.artifact}:${link.dataset.lineNumber}`);
  }
}

async function loadLine(artifact: string, line: number): Promise<boolean> {
  const showers = document.querySelectorAll<HTMLDivElement>(`.show-skipped[data-artifact="${artifact}"]`);
  for (const shower of Array.from(showers)) {
    if (line >= Number(shower.dataset.startLine) && line < Number(shower.dataset.endLine)) {
      // TODO(fejta): could maybe do something smarter here than the whole
      // block.
      await replaceElementWithContent(shower, 0, 0);
      return true;
    }
  }
  return false;
}

// parseHash extracts an artifact and line range.
//
// Expects URL fragment to be any of the following forms:
// * <empty>
// * single line: #artifact:5
// * range of lines: #artifact:5-12.
function parseHash(): [string, number, number]|null {
  const hash = location.hash.substr(1);
  const colonPos = hash.lastIndexOf(':');
  if (colonPos === -1) {
    return null;
  }
  const artifact = hash.substring(0, colonPos);
  const lineRange = hash.substring(colonPos + 1);
  const hyphenPos = lineRange.lastIndexOf('-');

  let startNum;
  let endNum;

  if (hyphenPos > 0 ) {
    startNum = Number(lineRange.substring(0, hyphenPos));
    endNum = Number(lineRange.substring(hyphenPos + 1));
  } else {
    startNum = Number(lineRange);
    endNum = startNum;
  }
  if (isNaN(startNum) || isNaN(endNum)) {
    return null;
  }
  if (endNum < startNum) { // ensure start has the smallest value.
    [startNum, endNum] = [endNum, startNum];
  }
  return [artifact, startNum, endNum];
}

async function handleHash(): Promise<void> {
  const result = parseHash();
  if (!result) {
    return;
  }
  const [artifact, startNum, endNum] = result;

  let firstEl = null;
  for (let lineNum = startNum; lineNum <= endNum; lineNum++) {
    const lineId = `${artifact}:${lineNum}`;
    let lineEl = document.getElementById(lineId);
    if (!lineEl) {
      if (await loadLine(artifact, lineNum)) {
        lineEl = document.getElementById(lineId);
        if (!lineEl) {
          return;
        }
      } else {
        return;
      }
    }
    if (firstEl === null) {
      firstEl = lineEl;
      clearHighlightedLines();
    }
    highlightLine(lineEl);
  }

  if (firstEl !== null) {
    const top = firstEl.getBoundingClientRect().top + window.pageYOffset;
    spyglass.scrollTo(0, top).then();
  }
}

window.addEventListener('hashchange', () => handleHash());

window.addEventListener('load', () => {
  const shown = document.getElementsByClassName("shown");
  for (const child of Array.from(shown)) {
    child.innerHTML = ansiToHTML(child.innerHTML);
  }

  for (const button of Array.from(document.querySelectorAll<HTMLDivElement>(".show-skipped"))) {
    button.addEventListener('click', handleShowSkipped);
    button.className = button.className + " showable";
  }

  for (const button of Array.from(document.querySelectorAll<HTMLButtonElement>("button.show-all-button"))) {
    button.addEventListener('click', handleShowAll);
  }

  for (const container of Array.from(document.querySelectorAll<HTMLElement>('.loglines'))) {
    container.addEventListener('click', handleLineLink, {capture: true});
  }
  fixLinks(document.documentElement);

  handleHash();
});
