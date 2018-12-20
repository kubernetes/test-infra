function toggleExpansion(dataId: string, textId: string, iconId: string): void {
  const body = document.getElementById(dataId)!;
  body.classList.toggle('hidden');
  const icon = document.getElementById(iconId)!;
  const textElem = document.getElementById(textId)!;
  if (body.classList.contains('hidden')) {
    icon.innerHTML = 'expand_more';
    textElem.innerText = 'Show more'
  } else {
    icon.innerHTML = 'expand_less';
    textElem.innerText = 'Show less'
  }
  spyglass.contentUpdated();
}

(window as any).toggleExpansion = toggleExpansion;
