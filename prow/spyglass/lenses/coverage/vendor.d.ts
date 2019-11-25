/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// For exciting Bazel/rollup reasons (ugh), it is impossible to `import "pako"`, which
// is the intended way to use this library. However, we *can*
// `import "pako/lib/inflate". Unfortunately, @types/pako doesn't define this
// module, so that then becomes a type error. This is a subset of @types/pako
// that we actually care about,
declare module "pako/lib/inflate" {
  export = Pako;
  namespace Pako {
    interface InflateFunctionOptions {
      windowBits?: number;
      raw?: boolean;
      to?: 'string';
    }

    type Data = Uint8Array | number[] | string;

    /**
     * Decompress data with inflate/ungzip and options. Autodetect format via wrapper header
     * by default. That's why we don't provide separate ungzip method.
     */
    function inflate(data: Data, options: InflateFunctionOptions & { to: 'string' }): string;
    function inflate(data: Data, options?: InflateFunctionOptions): Uint8Array;

    /**
     * The same as inflate, but creates raw data, without wrapper (header and adler32 crc).
     */
    function inflateRaw(data: Data, options: InflateFunctionOptions & { to: 'string' }): string;
    function inflateRaw(data: Data, options?: InflateFunctionOptions): Uint8Array;

    /**
     * Just shortcut to inflate, because it autodetects format by header.content. Done for convenience.
     */
    function ungzip(data: Data, options: InflateFunctionOptions & { to: 'string' }): string;
    function ungzip(data: Data, options?: InflateFunctionOptions): Uint8Array;
  }
}
