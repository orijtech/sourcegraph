// @ts-check
const path = require('path')

const config = require('../../../jest.config.base')

/** @type {jest.InitialOptions} */
const exportedConfig = {
  ...config,
  displayName: 'web-server',
  rootDir: __dirname,
  roots: ['<rootDir>'],
  setupFiles: [
    ...config.setupFiles,
    path.join(__dirname, 'jestSetup.ts'),
  ]
}

module.exports = exportedConfig
