import {isTransitMessage, serialiseHashes} from "./common";

declare const src: string;
declare const lensArtifacts: {[index: string]: string[]};
declare const lensIndexes: number[];
declare const csrfToken: string;

// Loads views for this job
function loadLenses(): void {
  const hashes = parseHash();
  for (const lensIndex of lensIndexes) {
    const frame = document.querySelector<HTMLIFrameElement>(`#iframe-${lensIndex}`)!;
    let url = urlForLensRequest(frame.dataset.lensName!, Number(frame.dataset.lensIndex!), 'iframe');
    url += `&topURL=${escape(location.href.split('#')[0])}&lensIndex=${lensIndex}`;
    const hash = hashes[lensIndex];
    if (hash) {
      url += hash;
    }
    frame.src = url;
  }
}

function queryForLens(lens: string, index: number): string {
  const data = {
    artifacts: lensArtifacts[index],
    index,
    src,
  };
  return `req=${encodeURIComponent(JSON.stringify(data))}`;
}

function urlForLensRequest(lens: string, index: number, request: string): string {
  return `/spyglass/lens/${lens}/${request}?${queryForLens(lens, index)}`;
}

function frameForMessage(e: MessageEvent): HTMLIFrameElement {
  for (const frame of Array.from(document.querySelectorAll('iframe'))) {
    if (frame.contentWindow === e.source) {
      return frame;
    }
  }
  throw new Error("MessageEvent from unknown frame!?");
}

function updateHash(index: number, hash: string): void {
  const currentHash = parseHash();
  if (hash !== '') {
    currentHash[index] = hash;
  } else {
    delete currentHash[index];
  }
  location.hash = serialiseHashes(currentHash);
}

function parseHash(): {[index: string]: string} {
  const parts = location.hash.substr(1).split(';');
  const result: { [index: string]: string } = {};
  for (const part of parts) {
    if (part === '') {
      continue;
    }
    const [index, hash] = part.split(':');
    result[index] = '#' + unescape(hash);
  }
  return result;
}

function getLensRequestOptions(reqBody: string): RequestInit {
  return {body: reqBody, method: 'POST', headers: {'X-CSRF-Token': csrfToken}, credentials: 'same-origin'};
}

window.addEventListener('message', async (e) => {
  const data = e.data;
  if (isTransitMessage(data)) {
    const {id, message} = data;
    const frame = frameForMessage(e);
    const lens = frame.dataset.lensName!;
    const index = Number(frame.dataset.lensIndex!);

    const respond = (response: string): void => {
      frame.contentWindow!.postMessage({id, message: {type: 'response', data: response}}, '*');
    };

    switch (message.type) {
      case "contentUpdated":
        frame.style.height = `${message.height}px`;
        frame.style.visibility = 'visible';
        if (frame.dataset.hideTitle) {
          frame.parentElement!.parentElement!.classList.add('hidden-title');
        }
        document.querySelector<HTMLElement>(`#${lens}-loading`)!.style.display = 'none';
        respond('');
        break;
      case "request": {
        const req = await fetch(urlForLensRequest(lens, index, 'callback'),
          getLensRequestOptions(message.data));
        respond(await req.text());
        break;
      }
      case "requestPage": {
        const req = await fetch(urlForLensRequest(lens, index, 'rerender'),
          getLensRequestOptions(message.data));
        respond(await req.text());
        break;
      }
      case "updatePage": {
        const spinner = document.querySelector<HTMLElement>(`#${lens}-loading`)!;
        frame.style.visibility = 'visible';
        spinner.style.display = 'block';
        const req = await fetch(urlForLensRequest(lens, index, 'rerender'),
          getLensRequestOptions(message.data));
        respond(await req.text());
        break;
      }
      case "updateHash": {
        updateHash(index, message.hash);
        respond('');
        break;
      }
      case "showOffset": {
        const container = document.getElementsByTagName('main')[0]!;
        const containerOffset = {left: 0, top: 0};
        let parent: HTMLElement = frame;
        // figure out our cumulative offset from the root container <main> by
        // looping through our parents until we get to it or run out of parents.
        while (parent) {
          containerOffset.top += parent.offsetTop;
          containerOffset.left += parent.offsetLeft;
          if (parent.offsetParent instanceof HTMLElement && parent.offsetParent !== container) {
            parent = parent.offsetParent;
          } else {
            break;
          }
        }
        if (!parent) {
          console.error("Couldn't find parent for frame!", container, frame);
        }
        container.scrollTop = containerOffset.top + message.top;
        container.scrollLeft = containerOffset.left + message.left;
        break;
      }
      default:
        console.warn(`Unrecognised message type "${message.type}" from lens "${lens}":`, data);
        break;
    }
  }
});

window.addEventListener('hashchange', (e) => {
  const hashes = parseHash();
  for (const index of Object.keys(hashes)) {
    const iframe = document.querySelector<HTMLIFrameElement>(`#iframe-${index}`);
    if (!iframe || !iframe.contentWindow) {
      continue;
    }
    iframe.contentWindow.postMessage({type: 'hashUpdate', hash: hashes[index]}, '*');
  }
});

// We can't use DOMContentLoaded here or we end up with a bunch of flickering. This appears to be MDL's fault.
window.addEventListener('load', () => {
    loadLenses();
});
