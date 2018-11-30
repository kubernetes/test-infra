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

async function handleShowSkipped(this: HTMLDivElement, e: MouseEvent) {
  // Don't do anything unless they actually clicked the button.
  if (!(e.target instanceof HTMLButtonElement)) {
    return;
  }
  const {artifact, offset, length, startLine} = this.dataset;
  const content = await spyglass.request(JSON.stringify({
    artifact, offset: +offset!, length: +length!, startLine: +startLine!}));
  this.innerHTML = ansiToHTML(content);
  showElem(this);

  // Remove the "show all" button if we no longer need it.
  const log = document.getElementById(`${artifact}-content`)!;
  const skipped = log.querySelectorAll<HTMLElement>(".show-skipped");
  if (skipped.length === 0) {
    const button = document.querySelector('button.show-all-button')!;
    button.parentNode!.removeChild(button);
  }
  spyglass.contentUpdated();
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

window.addEventListener('load', () => {
  const shown = document.getElementsByClassName("shown");
  for (let i = 0; i < shown.length; i++) {
    shown[i].innerHTML = ansiToHTML(shown[i].innerHTML);
  }

  for (const button of Array.from(document.querySelectorAll<HTMLDivElement>(".show-skipped"))) {
    button.addEventListener('click', handleShowSkipped);
  }

  for (const button of Array.from(document.querySelectorAll<HTMLButtonElement>("button.show-all-button"))) {
    button.addEventListener('click', handleShowAll);
  }
});
