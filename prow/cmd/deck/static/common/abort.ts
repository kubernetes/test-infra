import {ProwJobState} from "../api/prow";
import {showAlert, showToast, State} from "./common";

export function createAbortProwJobIcon(state: ProwJobState, prowjob: string, csrfToken: string): HTMLElement {
  const url = `${location.protocol}//${location.host}/abort?prowjob=${prowjob}`;
  const button = document.createElement('button');
  button.classList.add('mdl-button', 'mdl-js-button', 'mdl-button--icon');
  button.innerHTML = '<i class="icon-button material-icons" title="Cancel this job" style="color: gray">cancel</i>';

  if (state !== State.TRIGGERED && state !== State.PENDING) {
    button.innerHTML = `<i class="icon-button material-icons" title="Can't abort job in ${state} state" style="color: lightgray">cancel</i>`;
    button.classList.remove('icon-button');
    button.disabled = true;
  }

  button.onclick = async () => {
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

  return button;
}
