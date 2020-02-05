window.addEventListener('load', () => {
  document.querySelectorAll<HTMLAnchorElement>('a.mdl-tabs__tab').forEach((e) => {
    e.onclick = () => spyglass.contentUpdated();
  });

  document.querySelectorAll<HTMLAnchorElement>('a.expand-prow').forEach((e) => {
    e.onclick = () => {
      if (!e.parentElement || !e.parentElement.parentElement) {
        return;
      }
      e.parentElement.parentElement.classList.add("unhide");
      spyglass.contentUpdated();
    };
  });
});
