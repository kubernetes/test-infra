import {JobState} from "../api/prow";
import moment from "moment";

// The cell namespace exposes functions for constructing common table cells.
export namespace cell {

	export function text(text: string): HTMLTableDataCellElement {
		const c = document.createElement("td");
		c.appendChild(document.createTextNode(text));
		return c;
	};

	export function time(id: string, time: number): HTMLTableDataCellElement {
		const momentTime = moment.unix(time);
		const tid = "time-cell-" + id;
		const main = document.createElement("div");
		const isADayOld = momentTime.isBefore(moment().startOf('day'));
		main.textContent = momentTime.format(isADayOld ? 'MMM DD HH:mm:ss' : 'HH:mm:ss');
		main.id = tid;

		const tooltip = document.createElement("div");
		tooltip.textContent = momentTime.format('MMM DD YYYY, HH:mm:ss [UTC]ZZ');
		tooltip.setAttribute("data-mdl-for", tid);
		tooltip.classList.add("mdl-tooltip", "mdl-tooltip--large");

		const c = document.createElement("td");
		c.appendChild(main);
		c.appendChild(tooltip);

		return c;
	};

	export function link(text: string, url: string): HTMLTableDataCellElement {
		const c = document.createElement("td");
		const a = document.createElement("a");
		a.href = url;
		a.appendChild(document.createTextNode(text));
		c.appendChild(a);
		return c;
	};

	export function state(state: JobState): HTMLTableDataCellElement {
		const c = document.createElement("td");
		if (!state) {
			c.appendChild(document.createTextNode(""));
			return c;
		}
		c.classList.add("icon-cell");

		let displayState = stateToAdj(state);
		displayState = displayState[0].toUpperCase() + displayState.slice(1);
		let displayIcon = "";
		switch (state) {
			case "triggered":
				displayIcon = "schedule";
				break;
			case "pending":
				displayIcon = "watch_later";
				break;
			case "success":
				displayIcon = "check_circle";
				break;
			case "failure":
				displayIcon = "error";
				break;
			case "aborted":
				displayIcon = "remove_circle";
				break;
			case "error":
				displayIcon = "warning";
				break;
		}
		const stateIndicator = document.createElement("i");
		stateIndicator.classList.add("material-icons", "state", state);
		stateIndicator.innerText = displayIcon;
		c.appendChild(stateIndicator);
		c.title = displayState;

		return c;
	};

	function stateToAdj(state: JobState): string {
		switch (state) {
			case "success":
				return "succeeded";
			case "failure":
				return "failed";
			default:
				return state;
		}
	};

	export function commitRevision(repo: string, ref: string, SHA: string): HTMLTableDataCellElement {
		const c = document.createElement("td");
		const bl = document.createElement("a");
		bl.href = "https://github.com/" + repo + "/commit/" + SHA;
		bl.text = ref + " (" + SHA.slice(0, 7) + ")";
		c.appendChild(bl);
		return c;
	}

	export function prRevision(repo: string, num: number, author: string, title: string, SHA: string): HTMLTableDataCellElement {
		const td = document.createElement("td");
		addPRRevision(td, repo, num, author, title, SHA);
		return td;
	}

	let idCounter = 0;
	function nextID(): String {
	  idCounter++;
	  return "tipID-" + String(idCounter);
	};

	export function addPRRevision(elem: Node, repo: string, num: number, author: string, title: string, SHA: string): void {
		elem.appendChild(document.createTextNode("#"));
		const pl = document.createElement("a");
		pl.href = "https://github.com/" + repo + "/pull/" + num;
		pl.text = num.toString();
		if (title) {
			pl.id = "pr-" + repo + "-" + num + "-" + nextID();
			const tip = tooltip.forElem(pl.id, document.createTextNode(title));
			pl.appendChild(tip);
		}
		elem.appendChild(pl);
		if (SHA) {
			elem.appendChild(document.createTextNode(" ("));
			const cl = document.createElement("a");
			cl.href = "https://github.com/" + repo + "/pull/" + num
					+ '/commits/' + SHA;
			cl.text = SHA.slice(0, 7);
			elem.appendChild(cl);
			elem.appendChild(document.createTextNode(")"));
		}
		if (author) {
			elem.appendChild(document.createTextNode(" by "))
			const al = document.createElement("a");
			al.href = "https://github.com/" + author;
			al.text = author;
			elem.appendChild(al);
		}
	}
}

export namespace tooltip {
	export function forElem(elemID: string, tipElem: Node): Node {
		const tooltip = document.createElement("div");
		tooltip.appendChild(tipElem);
		tooltip.setAttribute("data-mdl-for", elemID);
		tooltip.classList.add("mdl-tooltip", "mdl-tooltip--large");
		return tooltip;
	}
}