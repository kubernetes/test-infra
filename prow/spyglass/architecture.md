# Spyglass Architecture

Spyglass is split into two major parts: the Spyglass core, and a set of independent lenses.
Lenses are designed to run statelessly and without any knowledge of the world outside being
provided with a list of artifacts. The core is responsible for selecting lenses and providing them
with artifacts.

## Spyglass Core

The Spyglass Core is split across [`prow/spyglass`](.) and [`prow/cmd/deck`](../cmd/deck). It has
the following responsibilities:

- Looking up artifacts for a given job and mapping those to lenses
- Generating a page that loads the required lenses
- Framing lenses with their boilerplate
- Faciliating communication between the lens frontends and backends

## Spyglass Lenses

Spyglass Lenses currently all live in [`prow/spyglass/lenses`](./lenses), though hopefully in the
future they can live elsewhere. Spyglass lenses have the following responsibilities:

- Fetching artifacts
- Rendering HTML for human consumption

Lens frontends are run in sandboxed iframes (currently `sandbox="allow-scripts allow-top-navigation
allow-popups"`), which ensures that they can only interact with the world via the intended API. In
particular, this prevents lenses from interacting with other Deck pseudo-APIs or with the core
spyglass page.

In order to provide this API to lenses, a library
([`prow/cmd/deck/static/spyglass/lens.ts`](../cmd/deck/static/spyglass/lens.ts)) is injected into
the lenses under the `spyglass` namespace. This library communicates with the spyglass core via
[`window.postMessage`](https://developer.mozilla.org/en-US/docs/Web/API/Window/postMessage). The
spyglass core then takes the requested action on the lens's behalf, which includes facilitating
communication between the lens frontend and backend. The messages exchanged between the core and the
lens are described in [`prow/cmd/deck/static/spyglass/common.ts`](../cmd/deck/static/spyglass/common.ts).
The messages are exchanged over a simple JSON-encoded protocol where each message sent from the lens
has an ID number attached, and a response with the same ID number is expected to be received.

For the purposes of static typing, the lens library is ambiently declared in
[`spyglass/lenses/lens.d.ts`](./lenses/lens.d.ts), which just re-exports the definition of
`spyglass` from `lens.ts`.

This design is discussed in its [implementation PR](https://github.com/kubernetes/test-infra/pull/10208).

### Lens endpoints

Lenses are exposed by the spyglass core on the following Deck endpoints:

| URL | Method | Purpose |
|---|---|---|
| `/spyglass/lens/:lens_name/iframe` | GET | The iframe view loaded directly by the spyglass core |
| `/spyglass/lens/:lens_name/rerender` | POST | Returns the lens `body`, used by calls to `spyglass.updatePage` and `spyglass.requestPage` |
| `/spyglass/lens/:lens_name/callback` | POST | Allows the lens frontend to exchange arbitrary strings with the lens backend. Used by `spyglass.request()` |

In all cases, the endpoint expects a JSON blob via the query parameter `req` that contains
bookkeeping information required by the spyglass core - the artifacts required, what job this is
about, a reference to the lens configuration. This information is attached to requests by the
spyglass core, and the lenses are not directly aware of it. In the case of the POSTed endpoints
`/rerender` and `/callback`, the lens can choose to attach an arbitrary string for its own use. This
string is passed through the core as an opaque string.

Some additional query parameters are attached to the iframes created by the spyglass core. These are
not used by the backend, and are provided as a convenient means to synchronously provide information
from the frontend core to the frontend lens library.

## Page loading sequence

When a spyglass page is loaded, the following occurs:

1. The **core** backend generates a list of artifacts for the job (e.g. by listing from GCS)
1. The **core** backend matches the artifact list against the configured lenses and determines which ones to
   display.
1. The **core** backend generates an HTML page with the lens->resource mapping embedded in it as JavaScript
   objects.
1. The **core** frontend reads the embedded mapping and generates iframes for each lens
1. The **core** receives the simultaneous requests to the lens endpoints and invokes the **lenses**
   to generate their content, injecting the lens library alongside some basic styling.

After this final step completes, the page is fully rendered. Lenses may choose to request additional
information from their frontend, in which case the following happens:

1. The **lens** frontend makes a request to the **core** frontend
1. The **core** frontend attaches some lens-specific metadata and makes an HTTP request to the
   relevant lens endpoint
1. The **core** backend receives the request and invokes the **lens** backend with the relevant
   information attached.
 