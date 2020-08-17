import {parseQuery} from '../common/urls';
import {isResponse, isTransitMessage, isUpdateHashMessage, Message, Response, serialiseHashes} from './common';

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
  /**
   * Returns a top-level URL that will cause your lens to be loaded with the
   * specified fragment. This is useful to construct copyable links, but generally
   * should not be used for immediate navigation.
   * @param fragment The fragment you want. If not prefixed with a #, one will
   *                 be assumed.
   */
  makeFragmentLink(fragment: string): string;

  /**
   * Scrolls the parent window so that the specified coordinates are visible.
   *
   * @param x The x coordinate relative to the lens document to scroll to.
   * @param y The y coordinate relative to the lens document to scroll to.
   */
  scrollTo(x: number, y: number): Promise<void>;
}

class SpyglassImpl implements Spyglass {
  private pendingRequests = new Map<number, (v: Response) => void>();
  private messageId = 0;
  private pendingUpdateTimer = 0;
  private currentHash = '';
  private observer: MutationObserver;

  constructor() {
    this.currentHash = location.hash;
    this.observer = new MutationObserver((mutations) => this.handleMutations(mutations));

    window.addEventListener('message', (e) => this.handleMessage(e));
    window.addEventListener('hashchange', (e) => this.handleHashChange(e));
    window.addEventListener('DOMContentLoaded', () => {
      this.fixAnchorLinks(document.documentElement);
      this.observer.observe(document.documentElement, {attributeFilter: ['href'], childList: true, subtree: true});
    });
    window.addEventListener('load', () => {
      this.contentUpdated();
      // this needs a delay but I'm not sure what (if anything) we're racing.
      setTimeout(() => {
        if (location.hash !== '') {
          this.tryMoveToHash(location.hash);
        }
      }, 100);
    });
  }

  public async updatePage(data: string): Promise<void> {
    await this.postMessage({type: 'updatePage', data});
    this.contentUpdated();
  }
  public async requestPage(data: string): Promise<string> {
    const result = await this.postMessage({type: 'requestPage', data});
    return result.data;
  }
  public async request(data: string): Promise<string> {
    const result = await this.postMessage({type: 'request', data});
    return result.data;
  }
  public contentUpdated(): void {
    this.updateHeight();
    clearTimeout(this.pendingUpdateTimer);
    // to be honest I have zero understanding of why this helps, but apparently it does.
    this.pendingUpdateTimer = setTimeout(() => this.updateHeight(), 0);
  }

  public makeFragmentLink(fragment: string): string {
    const q = parseQuery(location.search.substr(1));
    const topURL = q.topURL!;
    const lensIndex = q.lensIndex!;
    if (fragment[0] !== '#') {
      fragment = '#' + fragment;
    }
    return `${topURL}#${serialiseHashes({[lensIndex]: fragment})}`;
  }

  public async scrollTo(x: number, y: number): Promise<void> {
    await this.postMessage({type: 'showOffset', left: x, top: y});
  }

  private updateHeight(): void {
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

  private handleMessage(e: MessageEvent): void {
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
    } else if (isUpdateHashMessage(data)) {
      location.hash = data.hash;
    }
  }

  // When any links on the page are added or mutated, we fix them up if they
  // were anchor links to avoid developer confusion.
  private handleMutations(mutations: MutationRecord[]): void {
    for (const mutation of mutations) {
      if (!(mutation.target instanceof HTMLElement)) {
        continue;
      }
      if (mutation.type === 'childList') {
        this.fixAnchorLinks(mutation.target);
      } else if (mutation.type === 'attributes') {
        if (mutation.target instanceof HTMLAnchorElement && mutation.attributeName === 'href') {
          const href = mutation.target.getAttribute('href');
          if (href && href[0] === '#') {
            this.fixAnchorLink(mutation.target);
          }
        }
      }
    }
  }

  private handleHashChange(e: HashChangeEvent): void {
    if (location.hash === this.currentHash) {
      return;
    }
    this.currentHash = location.hash;
    this.postMessage({type: 'updateHash', hash: location.hash}).then();
    this.tryMoveToHash(location.hash);
  }

  // Because we're in an iframe that is exactly our height, anchor links don't
  // actually do anything (and even if they did, it would not be something
  // useful). We implement their intended behaviour manually by looking up the
  // element referred to and requesting that our parent jump to that offset.
  private tryMoveToHash(hash: string): void {
    hash = hash.substr(1);
    let el = document.getElementById(hash);
    if (!el) {
      el = document.getElementsByName(hash)[0];
      if (!el) {
        return;
      }
    }
    const top = el.getBoundingClientRect().top + window.pageYOffset;
    this.scrollTo(0, top).then();
  }

  // We need to fix up anchor links (i.e. links that only set the fragment)
  // because we use <base> to make asset references Just Work, but that also
  // applies to anchor links, which is surprising to developers.
  // In order to mitigate this surprise, we find all the links that were
  // supposed to be anchor links and fix them by adding the absolute URL
  // of the current page.
  private fixAnchorLinks(parent: Element): void {
    for (const a of Array.from(parent.querySelectorAll<HTMLAnchorElement>('a[href^="#"]'))) {
      this.fixAnchorLink(a);
    }
  }

  private fixAnchorLink(a: HTMLAnchorElement): void {
    if (!a.dataset.preserveAnchor) {
      a.href = location.href.split('#')[0] + a.getAttribute('href');
      a.target = "_self";
    }
  }
}

const spyglass = new SpyglassImpl();
(window as any).spyglass = spyglass;
