import { isTransitMessage } from "./common";

declare const src: string;
declare const lensArtifacts: {[key: string]: string[]};
declare const lensIndexes: number[];

// Loads views for this job
function loadLenses(): void {
  for (const lensIndex of lensIndexes) {
    const frame = document.querySelector<HTMLIFrameElement>(`#iframe-${lensIndex}`)!;
    frame.src = urlForLensRequest(frame.dataset.lensName!, Number(frame.dataset.lensIndex!), 'iframe');
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
          {body: message.data, method: 'POST'});
        respond(await req.text());
        break;
      }
      case "requestPage": {
        const req = await fetch(urlForLensRequest(lens, index, 'rerender'),
          {body: message.data, method: 'POST'});
        respond(await req.text());
        break;
      }
      case "updatePage": {
        const spinner = document.querySelector<HTMLElement>(`#${lens}-loading`)!;
        frame.style.visibility = 'visible';
        spinner.style.display = 'block';
        const req = await fetch(urlForLensRequest(lens, index, 'rerender'),
          {body: message.data, method: 'POST'});
        respond(await req.text());
        break;
      }
      default:
        console.warn(`Unrecognised message type "${message.type}" from lens "${lens}":`, data);
        break;
    }
  }
});

// We can't use DOMContentLoaded here or we end up with a bunch of flickering. This appears to be MDL's fault.
window.addEventListener('load', () => {
    loadLenses();
});
