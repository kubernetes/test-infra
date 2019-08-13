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

async function replaceElementWithContent(element: HTMLDivElement) {
  const {artifact, offset, length, startLine} = element.dataset;
  const content = await spyglass.request(JSON.stringify({
    artifact, length: +length!, offset: +offset!, startLine: +startLine!}));
  element.innerHTML = ansiToHTML(content);
  fixLinks(element);
  showElem(element);

  // Remove the "show all" button if we no longer need it.
  const log = document.getElementById(`${artifact}-content`)!;
  const skipped = log.querySelectorAll<HTMLElement>(".show-skipped");
  if (skipped.length === 0) {
    const button = document.querySelector('button.show-all-button')!;
    button.parentNode!.removeChild(button);
  }
  spyglass.contentUpdated();
}

async function handleShowSkipped(this: HTMLDivElement, e: MouseEvent): Promise<void> {
  // Don't do anything unless they actually clicked the button.
  if (!(e.target instanceof HTMLButtonElement)) {
    return;
  }
  await replaceElementWithContent(this);
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
  location.hash = `#${el.dataset.artifact}:${el.dataset.lineNumber}`;
  e.preventDefault();
}

function highlightLine(element: HTMLElement): void {
  for (const oldEl of Array.from(document.querySelectorAll('.highlighted-line'))) {
    oldEl.classList.remove('highlighted-line');
  }
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
      await replaceElementWithContent(shower);
      return true;
    }
  }
  return false;
}

async function handleHash(): Promise<void> {
  const hash = location.hash.substr(1);
  const colonPos = hash.lastIndexOf(':');
  if (colonPos === -1) {
    return;
  }
  const artifact = hash.substring(0, colonPos);
  const lineNum = Number(hash.substring(colonPos + 1));
  if (isNaN(lineNum)) {
    return;
  }
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
  const top = lineEl.getBoundingClientRect().top + window.pageYOffset;
  highlightLine(lineEl);
  spyglass.scrollTo(0, top).then();
}

window.addEventListener('hashchange', () => handleHash());

window.addEventListener('load', () => {
  const shown = document.getElementsByClassName("shown");
  for (const child of Array.from(shown)) {
    child.innerHTML = ansiToHTML(child.innerHTML);
  }

  for (const button of Array.from(document.querySelectorAll<HTMLDivElement>(".show-skipped"))) {
    button.addEventListener('click', handleShowSkipped);
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
