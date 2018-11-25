import {Message, Response, isResponse, isTransitMessage} from './common';

export interface Spyglass {
  /**
   * Replaces the lens display with a new server-rendered page.
   * The returned promise will be resolved once the page has been updated.
   *
   * @param data Some data to pass back to the server. JSON encoding is
   *             recommended, but not required.
   */
  updatePage(data: string): Promise<void>;
  /**
   * Requests that the server re-render the lens with the provided data, and
   * returns a promise that will resolve with that HTML as a string.
   *
   * This is equivalent to updatePage(), except that the displayed content is
   * not automatically changed.
   * @param data Some data to pass back to the server. JSON encoding is
   *             recommended, but not required.
   */
  requestPage(data: string): Promise<string>;
  /**
   * Sends a request to the server-side lens backend with the provided data, and
   * returns a promise that will resolve with the response as a string.
   *
   * @param data Some data to pass back to the server. JSON encoding is
   *             recommended, but not required.
   */
  request(data: string): Promise<string>;
  /**
   * Inform Spyglass that the lens content has updated. This should be called whenever
   * the visible content changes, so Spyglass can ensure that all content is visible.
   */
  contentUpdated(): void;
}

class SpyglassImpl implements Spyglass {
  private pendingRequests = new Map<number, (v: Response) => void>();
  private messageId = 0;

  constructor() {
    window.addEventListener('message', (e) => this.handleMessage(e));
  }

  async updatePage(data: string): Promise<void> {
    await this.postMessage({type: 'updatePage', data});
    this.contentUpdated();
  }
  async requestPage(data: string): Promise<string> {
    const result = await this.postMessage({type: 'requestPage', data});
    return result.data;
  }
  async request(data: string): Promise<string> {
    const result = await this.postMessage({type: 'request', data});
    return result.data;
  }
  contentUpdated(): void {
    // .then() to suppress complaints about unhandled promises (we just don't care here).
    this.postMessage({type: 'contentUpdated', height: document.body.offsetHeight}).then();
  }

  private postMessage(message: Message): Promise<Response> {
    return new Promise<Response>((resolve, reject) => {
      const id = ++this.messageId;
      this.pendingRequests.set(id, resolve);
      window.parent.postMessage({id, message}, document.location.origin);
    });
  }

  private handleMessage(e: MessageEvent) {
    if (e.origin !== document.location.origin) {
      console.warn(`Got MessageEvent from unexpected origin ${e.origin}; expected ${document.location.origin}`, e);
      return;
    }
    const data = e.data;
    if (isTransitMessage(data)) {
      if (isResponse(data.message)) {
        if (this.pendingRequests.has(data.id)) {
          this.pendingRequests.get(data.id)!(data.message);
          this.pendingRequests.delete(data.id);
        }
      }
    }
  }
}

const spyglass = new SpyglassImpl();

window.addEventListener('load', () => {
  spyglass.contentUpdated();
});

(window as any).spyglass = spyglass;
