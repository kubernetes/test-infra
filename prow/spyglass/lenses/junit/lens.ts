function toggleExpansion(bodyId: string, expanderId: string): void {
  const body = document.getElementById(bodyId)!;
  body.classList.toggle('hidden-tests');
  if (body.classList.contains('hidden-tests')) {
    document.getElementById(expanderId)!.innerHTML = 'expand_more';
  } else {
    document.getElementById(expanderId)!.innerHTML = 'expand_less';
  }
  spyglass.contentUpdated();
}

(window as any).toggleExpansion = toggleExpansion;
