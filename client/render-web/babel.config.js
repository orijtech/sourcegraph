// @ts-check

const { babelPresetEnvCommonOptions } = require('../../babel.config')
const jestConfig = require('../../jest.config.base')
jestConfig

/** @type {import('@babel/core').TransformOptions} */
const config = {
  extends: '../../babel.config.js',
  ignore: [
    // TODO(sqs): sync up with jest.config.base.js transformIgnorePatterns
    '../../node_modules/react-dom/**',
    '../../node_modules/react/**',
    '../../node_modules/mdi-react/**',
    '../../node_modules/monaco-editor/**',
  ],
  presets: [
    [
      '@babel/preset-env',
      {
        // This program is run with Node instead of in the browser, so we need to compile it to
        // CommonJS.
        modules: 'commonjs',
        ...babelPresetEnvCommonOptions,
      },
    ],
  ],
  "plugins": [
    ["css-modules-transform", {
      // TODO(sqs): sync up with webpack.config.js localIdentName
      "generateScopedName": "[name]__[local]_[hash:base64:5]",
      extensions: [".css", ".scss"],
      "camelCase": true,
    }],
  ]
}

module.exports = config
