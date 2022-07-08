import {copyToClipboardWithToast, icon} from "./common";
import {relativeURL} from "./urls";

export function createRerunProwJobIcon(modal: HTMLElement, rerunElement: HTMLElement, prowjob: string, rerunCreatesJob: boolean, csrfToken: string): HTMLElement {
  const url = `${location.protocol}//${location.host}/rerun?prowjob=${prowjob}`;
  const i = icon.create("refresh", "Show instructions for rerunning this job");

  window.onkeydown = (event: any) => {
    if ( event.key === "Escape" ) {
      modal.style.display = "none";
    }
  };
  window.onclick = (event: any) => {
    if (event.target === modal) {
      modal.style.display = "none";
    }
  };

  // we actually want to know whether the "access-token-session" cookie exists, but we can't always
  // access it from the frontend. "github_login" should be set whenever "access-token-session" is
  i.onclick = () => {
    modal.style.display = "block";
    rerunElement.innerHTML = `kubectl create -f "<a href="${url}">${url}</a>"`;
    const copyButton = document.createElement('a');
    copyButton.className = "mdl-button mdl-js-button mdl-button--icon";
    copyButton.onclick = () => copyToClipboardWithToast(`kubectl create -f "${url}"`);
    copyButton.innerHTML = "<i class='material-icons state triggered' style='color: gray'>file_copy</i>";
    rerunElement.appendChild(copyButton);
    if (rerunCreatesJob) {
        const runButton = document.createElement('a');
        runButton.innerHTML = "<button class='mdl-button mdl-js-button mdl-button--raised mdl-button--colored'>Rerun</button>";
        runButton.onclick = async () => {
            gtag("event", "rerun", {
                event_category: "engagement",
                transport_type: "beacon",
            });
            const result = await fetch(url, {
                headers: {
                    "Content-type": "application/x-www-form-urlencoded; charset=UTF-8",
                    "X-CSRF-Token": csrfToken,
                },
                method: 'post',
            });
            const data = await result.text();
            if (result.status === 401) {
                window.location.href = window.location.origin + `/github-login?dest=${relativeURL({rerun: "gh_redirect"})}`;
            } else {
                rerunElement.innerHTML = data;
            }
        };
        rerunElement.appendChild(runButton);
    }
  };

  return i;
}