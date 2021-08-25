// @ts-check

const semver = require('semver')

/** @type {import('@babel/core').TransformOptions} */
const config = {
  extends: '../../babel.config.js',
   "presets": [
    [
      "@babel/preset-env",
      {
        "modules": 'auto',
        "targets": {
          "node": "current"
        },
        bugfixes: true,
        useBuiltIns: 'entry',
        include: [
          // Polyfill URL because Chrome and Firefox are not spec-compliant
          // Hostnames of URIs with custom schemes (e.g. git) are not parsed out
          'web.url',
          // URLSearchParams.prototype.keys() is not iterable in Firefox
          'web.url-search-params',
          // Commonly needed by extensions (used by vscode-jsonrpc)
          'web.immediate',
          // Always define Symbol.observable before libraries are loaded, ensuring interopability between different libraries.
          'esnext.symbol.observable',
          // Webpack v4 chokes on optional chaining and nullish coalescing syntax, fix will be released with webpack v5.
          '@babel/plugin-proposal-optional-chaining',
          '@babel/plugin-proposal-nullish-coalescing-operator',
        ],
        // See https://github.com/zloirock/core-js#babelpreset-env
        corejs: semver.minVersion(require('../../package.json').dependencies['core-js']),
      },
    ],
    "@babel/preset-react",
    "@babel/preset-typescript"
  ],
  "ignore": [/EnterpriseWebAp2p/, path => {
    console.log('X: ', path)
    return false
  }],
  "plugins": [
    ["transform-import-css", {
      // TODO(sqs): sync up with webpack.config.js localIdentName
      "generateScopedName": "[name]__[local]_[hash:base64:5]"
    }],
    [
      "babel-plugin-transform-require-ignore",
      {
        "extensions": [".scss"]
      }
    ],
    ['@babel/plugin-transform-typescript', { isTSX: true }]
  ]
}

module.exports = config
