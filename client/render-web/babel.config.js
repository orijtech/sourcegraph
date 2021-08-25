// @ts-check

/** @type {import('@babel/core').TransformOptions} */
const config = {
  extends: '../../babel.config.js',
   "presets": [
    [
      "@babel/preset-env",
      {
        "modules": "auto",
        "targets": {
          "node": "current"
        }
      }
    ],
    "@babel/preset-react",
    "@babel/preset-typescript"
  ],
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
    ]
  ]
}

module.exports = config
