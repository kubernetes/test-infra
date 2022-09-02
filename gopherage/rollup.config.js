import { nodeResolve } from '@rollup/plugin-node-resolve';
import commonjs from '@rollup/plugin-commonjs';
import { terser } from "rollup-plugin-terser";

export default {
  context: "window",
  input: './gopherage/cmd/html/static/_output/browser.js',
  output: [
    {
      file: 'bundle.js',
      format: 'esm',
    },
    {
      file: 'bundle.min.js',
      format: 'esm',
      plugins: [terser()]
    }
],
  plugins: [nodeResolve(), commonjs()]
};
