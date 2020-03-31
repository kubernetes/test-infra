function addSectionExpanders(): void {
  const expanders = document.querySelectorAll<HTMLTableRowElement>('tr.section-expander');
  for (const expander of Array.from(expanders)) {
    expander.onclick = () => {
      const tbody = expander.parentElement!.nextElementSibling!;
      const icon = expander.querySelector('i')!;
      if (tbody.classList.contains('hidden-tests')) {
        tbody.classList.remove('hidden-tests');
        icon.innerText = 'expand_less';
      } else {
        tbody.classList.add('hidden-tests');
        icon.innerText = 'expand_more';
      }
      spyglass.contentUpdated();
    };
  }
}

function addTestExpanders(): void {
  const rows = document.querySelectorAll<HTMLTableRowElement>('.failure-name,.flaky-name');
  for (const row of Array.from(rows)) {
    row.onclick = () => {
      const sibling = row.nextElementSibling!;
      const icon = row.querySelector('i')!;
      if (sibling.classList.contains('hidden')) {
        sibling.classList.remove('hidden');
        icon.innerText = 'expand_less';
      } else {
        sibling.classList.add('hidden');
        icon.innerText = 'expand_more';
      }
      spyglass.contentUpdated();
    };
  }
}

function addStdoutOpeners(): void {
  const links = document.querySelectorAll<HTMLAnchorElement>('a.open-stdout');
  for (const link of Array.from(links)) {
    link.onclick = (e) => {
      e.preventDefault();
      const text = (link.nextElementSibling! as HTMLElement).innerHTML;
      const blob = new Blob([`
      <head>
        <title>Logs</title>
      </head>
      <body style="background-color: #303030; color: white; font-family: monospace; white-space: pre-wrap;">${text}</body>`], {type: 'text/html'});
      window.open(URL.createObjectURL(blob));
    };
  }
}

function loaded(): void {
  addTestExpanders();
  addStdoutOpeners();
  addSectionExpanders();
}

window.addEventListener('DOMContentLoaded', loaded);
