import {copyToClipboard, icon, showAlert, showToast} from "./common";
import {relativeURL} from "./urls";

export function createRerunProwJobIcon(modal: HTMLElement, parentEl: Element, prowjob: string, showRerunButton: boolean, csrfToken: string): HTMLElement {
  const LATEST_JOB = 'latest';
  const ORIGINAL_JOB = 'original';
  const inrepoconfigURL = 'https://github.com/kubernetes/test-infra/blob/master/prow/inrepoconfig.md';
  const i = icon.create("refresh", "Show instructions for rerunning this job");

  const closeModal = (): void => {
    modal.style.display = "none";
    // Resets modal content. If removed, elements will be concatenated, causing duplicates.
    parentEl.classList.remove('rerun-content', 'abort-content');
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
  const getJobURL = (mode: string): string => {
    return `${location.protocol}//${location.host}/rerun?mode=${mode}&prowjob=${prowjob}`;
  };
  let commandURL = getJobURL(ORIGINAL_JOB);
  const getCommandDescription = (mode: string): string => {
    return `The command is for the ${mode} configuration. Admin permissions are required to rerun via kubectl.`;
  };
  const getCommand = (url: string): string => {
    return `kubectl create -f "${url}"`;
  };

  // we actually want to know whether the "access-token-session" cookie exists, but we can't always
  // access it from the frontend. "github_login" should be set whenever "access-token-session" is
  i.onclick = () => {
    modal.style.display = "block";
    // Add the styles for rerun modal
    parentEl.classList.add('rerun-content');

    parentEl.innerHTML = `
      <h2 class="rerunModal-title">Rerun ProwJob</h2>
      <p class="rerunModal-description">
        Below you can choose to rerun this ProwJob with either the original or latest configuration.
        <br><br>
        Note: Rerunning a ProwJob will create a new instance of a ProwJob. Any ProwJobs already running will not be interrupted.
        <a href="${inrepoconfigURL}" target="_blank">
          Inrepoconfig
          <i class='rerunModal-openOut material-icons state triggered' style='color: gray'>open_in_new</i>
        </a>
        is currently not supported with the latest configuration option.
      </p>
      <div class="rerunModal-radioButtonGroup">
        <div class="rerunModal-radioButtonRow">
          <label class="rerunModal-radioLabel mdl-radio mdl-js-radio mdl-js-ripple-effect" for="rerunOriginalOption">
            <input type="radio" id="rerunOriginalOption" class="rerunOriginalOption mdl-radio__button" name="rerunOptions" checked>
            <span class="mdl-radio__label">Original Configuration</span>
          </label>
          (<a href="${getJobURL(ORIGINAL_JOB)}" target="_blank">View YAML
          <i class="rerunModal-openOut material-icons state triggered" style="color: gray">open_in_new</i>
          </a>)
        </div>
        <div class="rerunModal-radioButtonRow">
          <label class="rerunModal-radioLabel mdl-radio mdl-js-radio mdl-js-ripple-effect" for="rerunLatestOption">
            <input type="radio" id="rerunLatestOption" class="rerunLatestOption mdl-radio__button" name="rerunOptions">
            <span class="mdl-radio__label">Latest Configuration</span>
          </label>
          (<a href="${getJobURL(LATEST_JOB)}" target="_blank">View YAML
          <i class="rerunModal-openOut material-icons state triggered" style="color: gray">open_in_new</i>
          </a>)
        </div>
      </div>
      <div class="rerunModal-accordion">
        <button class="rerunModal-accordionButton">
          <i class="rerunModal-expandIcon material-icons state triggered" style="color: gray">expand_more</i>
          kubectl command
        </button>
        <div class="rerunModal-accordionPanel">
          <div class="accordion-panel-content">
            <p class="rerunModal-commandDescription">${getCommandDescription(LATEST_JOB)}</p>
            <div class="rerunModal-commandContent">
              <div class="rerunModal-command">${getCommand(commandURL)}</div>
              <a class="rerunModal-copyButton mdl-button mdl-js-button mdl-button--icon">
                <i class="material-icons state triggered" style="color: gray">content_copy</i>
              </a>
            </div>
          </div>
        </div>
      </div>
    `;

    const latestOption = parentEl.querySelector('.rerunLatestOption');
    const command = parentEl.querySelector('.rerunModal-command');
    const commandDescription = parentEl.querySelector('.rerunModal-commandDescription');
    latestOption.addEventListener('click', () => {
      commandURL = getJobURL(LATEST_JOB);
      commandDescription.innerHTML = getCommandDescription(LATEST_JOB);
      command.innerHTML = getCommand(commandURL);
    });
    const originalOption = parentEl.querySelector('.rerunOriginalOption');
    originalOption.addEventListener('click', () => {
      commandURL = getJobURL(ORIGINAL_JOB);
      commandDescription.innerHTML = getCommandDescription(ORIGINAL_JOB);
      command.innerHTML = getCommand(commandURL);
    });
    const copyButton = parentEl.querySelector('.rerunModal-copyButton');
    copyButton.addEventListener('click', () => {
      copyToClipboard(getCommand(commandURL));
      showToast("Copied to clipboard");
    });
    const accordion = parentEl.querySelector('.rerunModal-accordionButton');
    const accordionPanel = parentEl.querySelector('.rerunModal-accordionPanel');
    const expandIcon = parentEl.querySelector('.rerunModal-expandIcon');
    accordion.addEventListener('click', () => {
      if (!accordionPanel.classList.contains('rerunModal-accordionPanel--expanded')) {
        accordionPanel.classList.add('rerunModal-accordionPanel--expanded');
        expandIcon.classList.add('rerunModal-expandIcon--expanded');
      } else {
        accordionPanel.classList.remove('rerunModal-accordionPanel--expanded');
        expandIcon.classList.remove('rerunModal-expandIcon--expanded');
      }
    });

    if (showRerunButton) {
      const runButton = document.createElement('a');
      runButton.innerHTML = "<button class='mdl-button mdl-js-button mdl-button--raised mdl-button--colored'>Rerun</button>";
      runButton.onclick = async () => {
        gtag("event", "rerun", {
          event_category: "engagement",
          transport_type: "beacon",
        });
        try {
          const result = await fetch(commandURL, {
            headers: {
              "Content-type": "application/x-www-form-urlencoded; charset=UTF-8",
              "X-CSRF-Token": csrfToken,
            },
            method: 'post',
          });
          if (result.status === 401) {
            window.location.href = `${window.location.origin  }/github-login?dest=${relativeURL({rerun: "gh_redirect"})}`;
          }
          const data = await result.text();
          if (result.status >= 400) {
            showAlert(data);
          } else {
            showToast(data);
          }
        } catch (e) {
          showAlert(`Could not send request to rerun job: ${e}`);
        }
      };
      parentEl.appendChild(runButton);
    }
  };

  return i;
}
