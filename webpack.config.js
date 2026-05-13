//@ts-check
'use strict';

const path = require('path');

/** @type {import('webpack').Configuration[]} */
module.exports = [
  // Extension client bundle
  {
    target: 'node',
    mode: 'none',
    entry: './src/extension.ts',
    output: {
      path: path.resolve(__dirname, 'dist'),
      filename: 'extension.js',
      libraryTarget: 'commonjs2',
    },
    externals: {
      vscode: 'commonjs vscode',
    },
    resolve: {
      extensions: ['.ts', '.js'],
    },
    module: {
      rules: [
        {
          test: /\.ts$/,
          exclude: /node_modules/,
          use: [{ loader: 'ts-loader' }],
        },
      ],
    },
    devtool: 'nosources-source-map',
    infrastructureLogging: { level: 'log' },
  },
  // Language server bundle
  {
    target: 'node',
    mode: 'none',
    entry: './src/server/server.ts',
    output: {
      path: path.resolve(__dirname, 'dist'),
      filename: 'server.js',
      libraryTarget: 'commonjs2',
    },
    externals: {
      vscode: 'commonjs vscode',
    },
    resolve: {
      extensions: ['.ts', '.js'],
    },
    module: {
      rules: [
        {
          test: /\.ts$/,
          exclude: /node_modules/,
          use: [{ loader: 'ts-loader', options: { configFile: 'tsconfig.server.json' } }],
        },
      ],
    },
    devtool: 'nosources-source-map',
    infrastructureLogging: { level: 'log' },
  },
];
