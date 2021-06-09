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

window.addEventListener('load', () => {
    for (const button of Array.from(document.querySelectorAll<HTMLButtonElement>("button.copy"))) {
        button.addEventListener('click', handleCopy);
    }
});
