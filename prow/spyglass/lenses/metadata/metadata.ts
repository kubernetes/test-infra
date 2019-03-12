import moment from "moment";

const DATE_FORMAT = 'YYYY-MM-DD HH:mm:ss ZZ';

function handleClick(e: MouseEvent): void {
  e.preventDefault();
  const button = document.getElementById('show-table-link')!;
  const table = document.getElementById('data-table')!;
  table.classList.toggle('hidden');
  if (table.classList.contains('hidden')) {
    button.innerText = 'more info';
  } else {
    button.innerText = 'less info';
  }
  spyglass.contentUpdated();
}

function getLocalStartTime(): void {
  document.getElementById('show-table-link')!.onclick = handleClick;
  const elem = document.getElementById("summary-start-time")!;
  elem.innerText = moment(elem.innerText, DATE_FORMAT).calendar().replace(/Last|Yesterday|Today|Tomorrow/,
      (m) => m.charAt(0).toLowerCase() + m.substr(1));
}

window.onload = getLocalStartTime;
