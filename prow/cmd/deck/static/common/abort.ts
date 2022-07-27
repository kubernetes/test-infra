import {ProwJobState} from "../api/prow";
import {showAlert, showToast, State} from "./common";

export function createAbortProwJobIcon(modal: HTMLElement, parentEl: Element, job: string, state: ProwJobState, prowjob: string, csrfToken: string): HTMLElement {
  const url = `${location.protocol}//${location.host}/abort?prowjob=${prowjob}`;
  const abortButton = document.createElement('button');
  abortButton.classList.add('mdl-button', 'mdl-js-button', 'mdl-button--icon');
  abortButton.innerHTML = '<i class="icon-button material-icons" title="Cancel this job" style="color: gray">cancel</i>';

  const closeModal = (): void => {
    modal.style.display = "none";
    // Resets modal content. If removed, elements will be concatenated, causing duplicates.
    parentEl.classList.remove('abort-content', 'rerun-content');
    parentEl.innerHTML = '';
  };
  window.onkeydown = (event: any) => {
    if (event.key === "Escape") {
      closeModal();
    }
  };
  window.onclick = (event: any) => {
    if (event.target === modal) {
      closeModal();
    }
  };
  if (state !== State.TRIGGERED && state !== State.PENDING) {
    abortButton.innerHTML = `<i class="icon-button material-icons" title="Can't abort job in ${state} state" style="color: lightgray">cancel</i>`;
    abortButton.disabled = true;
  }
  abortButton.onclick = async () => {
    modal.style.display = 'block';
    // Add the styles for abort modal
    parentEl.classList.add('abort-content');
    parentEl.innerHTML = `
      <h2 class="abortModal-title">Abort ProwJob</h2>
      <p class="abortModal-description">Would you like to abort <b>${job}</b>?</p>
    `;

    const buttonDiv = document.createElement('div');
    buttonDiv.classList.add('abortModal-buttonDiv');
    const confirmAbortButton = document.createElement('a');
    confirmAbortButton.innerHTML = "<button class='mdl-button mdl-js-button mdl-button--raised mdl-button--colored'>Confirm</button>";
    const cancelAbortButton = document.createElement('a');
    cancelAbortButton.innerHTML = "<button class='mdl-button mdl-js-button mdl-button--raised mdl-color--red mdl-button--colored'>Cancel</button>";
    buttonDiv.appendChild(confirmAbortButton);
    buttonDiv.appendChild(cancelAbortButton);
    parentEl.appendChild(buttonDiv);

    confirmAbortButton.onclick = async () => {
      gtag('event', 'abort', {
        event_category: 'engagement',
        transport_type: 'beacon',
      });
      try {
        const result = await fetch(url, {
          headers: {
            'Content-type': 'application/x-www-form-urlencoded; charset=UTF-8',
            'X-CSRF-Token': csrfToken,
          },
          method: 'post',
        });
        const data = await result.text();
        if (result.status >= 400) {
          showAlert(data);
        } else {
          showToast(data);
        }
      } catch (e) {
        showAlert(`Could not send request to abort job: ${e}`);
      }
    };
    cancelAbortButton.onclick = closeModal;
  };
  return abortButton;
}
