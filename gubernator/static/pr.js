const upArrow = '▲';
const downArrow = '▼';

function sortRows(rows, col, colName, dir) {
  // customize sorting functions for different columns
  let extract = el => el.innerText.replace(/^\s+|\s+$/g, '');
  let fn = x => x;
  switch (colName.trim()) {
    case 'Number':
      fn = Number;
      break;
    case 'Size':
      fn = x => ['XS', 'S', 'M', 'L', 'XL', 'XXL'].indexOf(x);
      break;
    case 'Updated':
      extract = el => Number(el.firstChild.dataset.epoch);
      break;
    case 'Author':
    case 'Assignees':
      fn = x => x.toLowerCase();
      break;
  }

  for (let i = 0; i < rows.length; i++) {
    rows[i].index = i;
  }

  rows.sort(function(a, b) {
    let valA = fn(extract(a.children[col]));
    let valB = fn(extract(b.children[col]));
    if (valA === valB) {
      return a.index - b.index;  // make the sort stable
    } else if (valA < valB) {
      return -dir;
    } else {
      return dir;
    }
  });
}

function sortColumn(evt) {
  const target = evt.target;
  const reverse = target.innerText[0] == upArrow;

  let col, i = 0;

  // Remove all directional arrows.
  for (let sibling of target.parentElement.children) {
    const first = sibling.innerText[0];
    if (first == upArrow || first == downArrow) {
      sibling.innerText = sibling.innerText.slice(2);
    }
    if (sibling === target) {
      col = i;
    }
    i++;
  }

  const table = target.closest('table');
  let tbody = table.tBodies[0];
  const rows = Array.prototype.slice.call(tbody.children);
  tbody.remove();               // clear old rows
  tbody = table.createTBody();  // create a new body

  sortRows(rows, col, target.innerText, reverse ? -1 : 1);

  for (let row of rows) {
    tbody.appendChild(row);
  }

  // Add arrow.
  target.innerText = (reverse ? downArrow : upArrow) + ' ' + target.innerText;

  return true;
}

window.addEventListener('load', function() {
  for (let th of document.getElementsByTagName('TH')) {
    th.addEventListener('click', sortColumn, false);
  }
});
