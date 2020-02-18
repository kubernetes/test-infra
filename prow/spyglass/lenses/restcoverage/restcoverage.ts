function addReportExpander(): void {
    const reportBtn = document.querySelector<HTMLSpanElement>('span#report-expander')!;
    const report = document.querySelector<HTMLDivElement>('div#report')!;
    const icon = document.querySelector<HTMLElement>('div#report-brief i.material-icons')!;
    reportBtn.onclick = () => {
        if (report.classList.toggle('hidden')) {
            icon.textContent = "unfold_more";
        } else {
            icon.textContent = "unfold_less";
        }
        spyglass.contentUpdated();
    };
}

function addParamsExpander(): void {
    const methods = document.querySelectorAll<HTMLLIElement>('ul.methods li.method');
    for (const method of Array.from(methods)) {
        const icon = method.querySelector<HTMLElement>('i.material-icons')!;
        if (icon == null) {
            method.style.cursor = "default";
            continue;
        }
        const sibling = method.nextElementSibling!;
        method.onclick = () => {
            if (sibling.classList.toggle('hidden')) {
                icon.textContent = "arrow_drop_down";
            } else {
                icon.textContent = "arrow_drop_up";
            }
            spyglass.contentUpdated();
        };
    }
}

function loaded(): void {
    addReportExpander();
    addParamsExpander();
}

window.addEventListener('DOMContentLoaded', loaded);
