import {Spyglass} from "../../cmd/deck/static/spyglass/lens";

declare global {
  // The `spyglass` global is injected into the environment the lens runs in by spyglass.
  const spyglass: Spyglass;
}
