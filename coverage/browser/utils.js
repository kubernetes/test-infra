// JavaScript for some reason can map over arrays but nothing else.
// Provide our own tools.

export function* map(iterable, fn) {
  for (let entry of iterable) {
    yield fn(entry);
  }
}

export function reduce(iterable, fn, initialValue) {
  let accumulator = initialValue;
  for (let entry of iterable) {
    accumulator = fn(accumulator, entry);
  }
  return accumulator;
}

export function* filter(iterable, fn) {
  for (let entry of iterable) {
    if (fn(entry)) {
      yield entry;
    }
  }
}
