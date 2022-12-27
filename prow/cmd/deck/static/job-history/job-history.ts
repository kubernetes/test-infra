import moment from "moment";
import {cell, formatDuration} from '../common/common';

declare const allBuilds: any;

window.onload = (): void => {
  const tbody = document.getElementById("history-table-body")!;

  for (const build of allBuilds) {
    const tr = document.createElement("tr");

    let className = "";
    switch (build.Result) {
      case "SUCCESS":
        className = "run-success";
        break;
      case "FAILURE":
        className = "run-failure";
        break;
      case "ERROR":
        className = "run-error";
        break;
      case "ABORTED":
        className = "run-aborted";
        break;
      default:
        className = "run-pending";
    }
    tr.classList.add(className);

    tr.appendChild(cell.link(build.ID, build.SpyglassLink));

    if (build.Refs && build.Refs.pulls) {
      for (const pull of build.Refs.pulls) {
        tr.appendChild(cell.prRevision(`${build.Refs.org}/${build.Refs.repo}`, pull));
      }
    } else {
      tr.appendChild(cell.text(""));
    }

    const started = Date.parse(build.Started) / 1000;
    tr.appendChild(cell.time(build.ID, moment.unix(started)));
    tr.appendChild(cell.text(formatDuration(build.Duration / 1000000000 ))); // convert from ns to s.
    tr.appendChild(cell.text(build.Result));

    for (const child of tr.children) {
      child.classList.add("mdl-data-table__cell--non-numeric");
    }

    tbody.appendChild(tr);
  }
};
