import {copyToClipboardWithToast, icon} from "./common";
import {relativeURL} from "./urls";

export function createRerunProwJobIcon(modal: HTMLElement, rerunElement: HTMLElement, prowjob: string, rerunCreatesJob: boolean, csrfToken: string): HTMLElement {
  const LATEST = 'latest';
  const ORIGINAL = 'original';
  const latestURL = `${location.protocol}//${location.host}/rerun?mode=latest&prowjob=${prowjob}`;
  const originalURL = `${location.protocol}//${location.host}/rerun?mode=original&prowjob=${prowjob}`;
  let commandURL = latestURL;
  const i = icon.create("refresh", "Show instructions for rerunning this job");

  window.onkeydown = (event: any) => {
    if ( event.key === "Escape" ) {
      modal.style.display = "none";
      // Resets modal content. If removed elements will be concatenated, causing duplicates.
      rerunElement.innerHTML = '';
    }
  };
  window.onclick = (event: any) => {
    if (event.target === modal) {
      modal.style.display = "none";
      // Resets modal content. If removed elements will be concatenated, causing duplicates.
      rerunElement.innerHTML = '';
    }
  };
  
  const getCommandDescription = (mode: string): string => {
    return `The command is for the ${mode} configuration. Note: Admin permissions are required to rerun via kubectl.`
  }
  const getCommand = (url: string):string => {
    return `kubectl create -f "${url}"`
  };

  // we actually want to know whether the "access-token-session" cookie exists, but we can't always
  // access it from the frontend. "github_login" should be set whenever "access-token-session" is
  i.onclick = () => {
    modal.style.display = "block";

    rerunElement.innerHTML = `
      <h2 class="rerunModal-title">Rerun ProwJob</h2>
      <p class="rerunModal-description">
        Rerun the latest or original configuration. Rerunning a ProwJob will create a new one.
        The old ProwJob will not stop or cancel. Inrepoconfig is currently not supported for
        latest configuration.
      </p>
      <div class="rerunModal-radioButtonGroup">
        <div class="rerunModal-radioButtonRow">
          <label class="rerunModal-radioLabel mdl-radio mdl-js-radio mdl-js-ripple-effect" for="rerunLatestOption">
            <input type="radio" id="rerunLatestOption" class="mdl-radio__button" name="rerunOptions" checked>
            <span class="mdl-radio__label">Latest Configuration</span>
          </label>
          ${icon.createAsString('description', 'View latest job', null, latestURL)}
        </div>
        <div class="rerunModal-radioButtonRow">
          <label class="rerunModal-radioLabel mdl-radio mdl-js-radio mdl-js-ripple-effect" for="rerunOriginalOption">
            <input type="radio" id="rerunOriginalOption" class="mdl-radio__button" name="rerunOptions">
            <span class="mdl-radio__label">Original Configuration</span>
          </label>
          ${icon.createAsString('description', 'View original job', null, originalURL)}
        </div>
      </div>
      <div class="rerunModal-accordion">
        <button class="rerunModal-accordionButton">
        <i class='rerunModal-expandIcon material-icons state triggered' style='color: gray'>expand_more</i>
          Rerun Command
        </button>
        <div class="rerunModal-accordionPanel">
          <div class="accordion-panel-content">
            <p class="rerunModal-commandDescription">${getCommandDescription(LATEST)}</p>
            <div class="rerunModal-commandContent">
              <div class="rerunModal-command">${getCommand(commandURL)}</div>
              <a class="rerunModal-copyButton mdl-button mdl-js-button mdl-button--icon">
                <i class='material-icons state triggered' style='color: gray'>content_copy</i>
              </a>
            </div>
          </div>
        </div>
      </div>
    `;

    const rerunLatestOption = rerunElement.querySelector('#rerunLatestOption');
    const rerunCommand = rerunElement.querySelector('.rerunModal-command');
    const rerunCommandDescription = rerunElement.querySelector('.rerunModal-commandDescription');
    rerunLatestOption.addEventListener('click', () => {
      commandURL = latestURL;
      rerunCommandDescription.innerHTML = getCommandDescription(LATEST);
      rerunCommand.innerHTML = getCommand(commandURL);
    });
    const rerunOriginalOption = rerunElement.querySelector('#rerunOriginalOption');
    rerunOriginalOption.addEventListener('click', () => {
      commandURL = originalURL;
      rerunCommandDescription.innerHTML = getCommandDescription(ORIGINAL);
      rerunCommand.innerHTML = getCommand(commandURL);
    });
    const rerunCopyButton = rerunElement.querySelector('.rerunModal-copyButton');
    rerunCopyButton.addEventListener('click', () => {
      copyToClipboardWithToast(getCommand(commandURL));
    })
    const accordion = rerunElement.querySelector('.rerunModal-accordionButton');
    const accordionPanel = rerunElement.querySelector('.rerunModal-accordionPanel');
    const expandIcon = rerunElement.querySelector('.rerunModal-expandIcon');
    accordion.addEventListener('click', () => {
      if (!accordionPanel.classList.contains('rerunModal-accordionPanel--expanded')) {
        accordionPanel.classList.add('rerunModal-accordionPanel--expanded');
        expandIcon.classList.add('rerunModal-expandIcon--expanded');
      } else {
        accordionPanel.classList.remove('rerunModal-accordionPanel--expanded');
        expandIcon.classList.remove('rerunModal-expandIcon--expanded');
      }
    });

    if (rerunCreatesJob) {
        const runButton = document.createElement('a');
        runButton.innerHTML = "<button class='mdl-button mdl-js-button mdl-button--raised mdl-button--colored'>Rerun</button>";
        runButton.onclick = async () => {
            gtag("event", "rerun", {
                event_category: "engagement",
                transport_type: "beacon",
            });
            const result = await fetch(commandURL, {
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