import "jasmine";
import {enumerate, filter, map, reduce} from './utils';

describe('map', () => {
  it('should map over an array', () => {
    expect(Array.from(map([1, 3, 5], (x) => 2 * x))).toEqual([2, 6, 10]);
  });

  it('should call the function lazily', () => {
    const spy = jasmine.createSpy('mapper').and.callFake((x: number) => 2 * x);

    const [generatorSpy, input] = iterableSpy([1, 3]);
    const iterable = map(input, spy);
    const iterator = iterable[Symbol.iterator]();
    expect(generatorSpy).not.toHaveBeenCalled();
    expect(spy).not.toHaveBeenCalled();

    let next = iterator.next();
    expect(next.value).toBe(2);
    expect(spy).toHaveBeenCalledTimes(1);
    expect(spy).toHaveBeenCalledWith(1);
    expect(generatorSpy).toHaveBeenCalledTimes(1);

    next = iterator.next();
    expect(next.value).toBe(6);
    expect(spy).toHaveBeenCalledTimes(2);
    expect(spy).toHaveBeenCalledWith(3);
    expect(generatorSpy).toHaveBeenCalledTimes(2);

    next = iterator.next();
    expect(next.done).toBe(true);
    expect(spy).toHaveBeenCalledTimes(2);
    expect(generatorSpy).toHaveBeenCalledTimes(3);
  });

  it('should accept non-array iterables', () => {
    function* generator(): Iterable<number> {
      yield 1;
      yield 3;
    }
    expect(Array.from(map(generator(), (x) => x * 2))).toEqual([2, 6]);
  });

  it('should do nothing with empty input', () => {
    expect(Array.from(map([], (x) => x))).toEqual([]);
  });
});

describe('reduce', () => {
  it('should reduce non-array iterators', () => {
    function* generator(): Iterable<number> {
      yield 1;
      yield 2;
    }
    expect(reduce(generator(), (acc, x) => acc + x, 0)).toBe(3);
  });
});

describe('filter', () => {
  it('should accept and produce iterables', () => {
    const [inputSpy, input] = iterableSpy([1, 2, 3, 4]);

    const f = jasmine.createSpy('f').and.callFake((x: number) => x % 2 === 0);
    const iterable = filter(input, f);
    const iterator = iterable[Symbol.iterator]();

    expect(f).not.toHaveBeenCalled();
    let value = iterator.next();
    expect(value.value).toBe(2);
    expect(f).toHaveBeenCalledTimes(2);
    expect(inputSpy).toHaveBeenCalledTimes(2);

    value = iterator.next();
    expect(value.value).toBe(4);
    expect(f).toHaveBeenCalledTimes(4);
    expect(inputSpy).toHaveBeenCalledTimes(4);

    value = iterator.next();
    expect(value.done).toBe(true);
    expect(f).toHaveBeenCalledTimes(4);
  });
});

describe('enumerate', () => {
  it('should count up', () => {
    expect(Array.from(enumerate(['hello', 'world']))).toEqual([
      [0, 'hello'], [1, 'world'],
    ]);
  });

  it('should accept and produce iterables', () => {
    const [inputSpy, input] = iterableSpy(['hello', 'world']);

    const iterable = enumerate(input);
    const iterator = iterable[Symbol.iterator]();

    expect(inputSpy).not.toHaveBeenCalled();

    let value = iterator.next();
    expect(value.value).toEqual([0, 'hello']);
    expect(inputSpy).toHaveBeenCalledTimes(1);

    value = iterator.next();
    expect(value.value).toEqual([1, 'world']);
    expect(inputSpy).toHaveBeenCalledTimes(2);

    value = iterator.next();
    expect(value.done).toBe(true);
    expect(inputSpy).toHaveBeenCalledTimes(3);
  });
});

// Given an array, returns an iterable that yields the values one at a time,
// and also a Spy that lets you observe the usage of that iterable.
function iterableSpy<T>(output: T[]): [jasmine.Spy, Iterable<T>] {
  const iterator = {
    next(): IteratorResult<T> {
      if (output.length > 0) {
        return {value: output.shift()!, done: false};
      } else {
        // IteratorResult<T> is incorrect for finished iterators, apparently.
        return {done: true} as IteratorResult<T>;
      }
    },
  };

  const iterable = {
    [Symbol.iterator]() {
      return iterator;
    },
  };

  const spy = spyOn(iterator, 'next').and.callThrough();
  return [spy, iterable];
}
