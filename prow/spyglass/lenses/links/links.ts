// https://stackoverflow.com/a/49121680

function copyMessage(val: string) {
    const selBox = document.createElement('textarea');
    selBox.style.position = 'fixed';
    selBox.style.left = '0';
    selBox.style.top = '0';
    selBox.style.opacity = '0';
    selBox.value = val;
    document.body.appendChild(selBox);
    selBox.focus();
    selBox.select();
    document.execCommand('copy');
    document.body.removeChild(selBox);
}

async function handleCopy(this: HTMLButtonElement) {
    copyMessage(this.dataset.link || "");
}

function addLinksExpanders(): void {
    const expanders = document.querySelectorAll<HTMLTableRowElement>('tr.links-expander');
    for (const expander of Array.from(expanders)) {
        expander.onclick = () => {
            const tbody = expander.parentElement!.nextElementSibling!;
            const icon = expander.querySelector('i')!;
            if (tbody.classList.contains('hidden-links')) {
                tbody.classList.remove('hidden-links');
                icon.innerText = 'expand_less';
            } else {
                tbody.classList.add('hidden-links');
                icon.innerText = 'expand_more';
            }
            spyglass.contentUpdated();
        };
    }
}

window.addEventListener('load', () => {
    for (const button of Array.from(document.querySelectorAll<HTMLButtonElement>("button.copy"))) {
        button.addEventListener('click', handleCopy);
    }
});

window.addEventListener('DOMContentLoaded', addLinksExpanders);
