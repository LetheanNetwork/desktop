// SPDX-Licence-Identifier: EUPL-1.2

import { spawnSync } from 'node:child_process';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const managedKeys = [
  'EXTRA_TAGS',
  'LTHN_DEV',
  'LTHN_WAILS_WS_LISTEN',
  'LTHN_WAILS_WS_URL',
];
const managedKeySet = new Set(managedKeys.map((key) => key.toLowerCase()));

const commands = Object.freeze({
  build: Object.freeze({
    task: 'build',
    env: Object.freeze({ EXTRA_TAGS: 'mcp' }),
  }),
  run: Object.freeze({
    task: 'run',
    env: Object.freeze({
      LTHN_DEV: '1',
      LTHN_WAILS_WS_LISTEN: '127.0.0.1:9199',
      LTHN_WAILS_WS_URL: 'ws://localhost:9199/wails/ws',
    }),
  }),
});

function commandDefinition(command) {
  if (!Object.hasOwn(commands, command)) {
    throw new Error(`unknown development command: ${command}`);
  }
  return commands[command];
}

export function developmentCommandEnvironment(command, ambient = process.env) {
  const definition = commandDefinition(command);
  const environment = {};

  for (const [key, value] of Object.entries(ambient)) {
    if (!managedKeySet.has(key.toLowerCase())) {
      environment[key] = value;
    }
  }

  return { ...environment, ...definition.env };
}

export function runDevelopmentCommand(command, spawn = spawnSync, ambient = process.env) {
  const definition = commandDefinition(command);
  const result = spawn('wails3', ['task', definition.task], {
    env: developmentCommandEnvironment(command, ambient),
    stdio: 'inherit',
  });
  if (result.error) {
    throw result.error;
  }
  return result.status ?? 1;
}

const isMain =
  process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href;

if (isMain) {
  if (process.argv.length !== 3) {
    console.error('usage: wails-dev-command.mjs <build|run>');
    process.exitCode = 1;
  } else {
    try {
      process.exitCode = runDevelopmentCommand(process.argv[2]);
    } catch (error) {
      console.error(error instanceof Error ? error.message : String(error));
      process.exitCode = 1;
    }
  }
}
