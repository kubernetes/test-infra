import {tooltip} from '../common/common';

window.onload = (): void => {
  const rows = document.getElementsByClassName("build-row");

  for (const row of rows) {
    const title = row.getAttribute("data-title");
    if (title != null) {
      const tip = tooltip.forElem(row.id, document.createTextNode(title));
      tip.classList.add("mdl-tooltip--right");
      row.appendChild(tip);
    }
  }
};
