function addReportExpander(): void {
    const reportBtn = document.querySelector<HTMLSpanElement>('span#report-expander')!;
    reportBtn.onclick = () => {
        const report = document.querySelector<HTMLDivElement>('div#report')!;
        report.classList.toggle('hidden');
        spyglass.contentUpdated();
    }
}

function addParamsExpander(): void {
    const methods = document.querySelectorAll<HTMLLIElement>('ul.methods li.method');
    for (const method of Array.from(methods)) {
        method.onclick = () => {
            const sibling = method.nextElementSibling!;
            sibling.classList.toggle('hidden');
            method.classList.toggle('active');
            spyglass.contentUpdated();
        }
    }
}

function loaded(): void {
    addReportExpander();
    addParamsExpander();
}
  
window.addEventListener('DOMContentLoaded', loaded);