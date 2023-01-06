import { nodeResolve } from '@rollup/plugin-node-resolve';
import commonjs from '@rollup/plugin-commonjs';

const inputFile = process.env.ROLLUP_ENTRYPOINT;
const outputFile = process.env.ROLLUP_OUT_FILE;

export default {
  context: "window",
  input: inputFile,
  output: [
    {
      file: outputFile,
      format: 'esm'
    }
  ],
  plugins: [nodeResolve(), commonjs()]
};
