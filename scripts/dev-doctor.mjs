import { execFile as execFileCallback } from 'node:child_process';
import { access } from 'node:fs/promises';
import { createServer } from 'node:net';
import { homedir } from 'node:os';
import { dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const execFile = promisify(execFileCallback);
const scriptPath = fileURLToPath(import.meta.url);
const defaultRepoRoot = resolve(dirname(scriptPath), '..');

const COMMANDS = [
  {
    name: 'Node.js',
    command: process.execPath,
    args: ['--version'],
    hint: 'Install the Node.js version selected by the frontend toolchain.',
  },
  {
    name: 'npm',
    command: 'npm',
    args: ['--version'],
    hint: 'Install npm with Node.js.',
  },
  {
    name: 'Go',
    command: 'go',
    args: ['version'],
    hint: 'Install the Go toolchain declared by go/go.mod.',
  },
  {
    name: 'Wails',
    command: 'go',
    args: ['tool', 'wails3', 'version'],
    hint: 'Restore the Wails tool declared by go/go.mod with the project Go toolchain.',
  },
  {
    name: 'Task',
    command: 'task',
    args: ['--version'],
    hint: 'Install go-task and ensure task is on PATH.',
  },
];

const PORTS = [
  {
    port: 9099,
    name: 'Wails MCP (9099)',
    hint: 'Close the existing development app if a new Wails session must own this port.',
  },
  {
    port: 9199,
    name: 'Lethean WebSocket (9199)',
    hint: 'Close the existing Lethean development process before starting another one.',
  },
  {
    port: 9245,
    name: 'Angular HMR (9245)',
    hint: 'Stop the existing Angular development server or reuse that session.',
  },
];

export async function inspectDevelopmentEnvironment({
  cwd = defaultRepoRoot,
  homeDir = homedir(),
  environment = process.env,
  commandProbe = probeCommand,
  portProbe = probePort,
  pathProbe = probePath,
} = {}) {
  const commands = await Promise.all(
    COMMANDS.map(async (command) => {
      const result = await commandProbe(command);
      return {
        kind: 'command',
        name: command.name,
        status: result.available ? 'ok' : 'error',
        detail: result.detail,
        hint: result.available ? '' : command.hint,
      };
    }),
  );

  const paths = [
    {
      name: 'npm lockfile',
      path: join(cwd, 'frontend-ng', 'package-lock.json'),
      required: true,
      hint: 'Restore frontend-ng/package-lock.json before installing dependencies.',
    },
    {
      name: 'frontend dependencies',
      path: join(cwd, 'frontend-ng', 'node_modules'),
      required: false,
      hint: 'Run npm ci in frontend-ng or task common:install:frontend:deps.',
    },
    {
      name: 'generated Wails bindings',
      path: join(cwd, 'frontend-ng', 'bindings'),
      required: false,
      hint: 'Run task common:generate:bindings; task dev also generates them when needed.',
    },
    {
      name: 'go-mlx repository',
      path:
        environment.LTHN_MLX_REPO ||
        join(homeDir, 'Code', 'core', 'go-mlx'),
      required: false,
      hint: 'Optional: set LTHN_MLX_REPO when go-mlx lives elsewhere.',
    },
    {
      name: 'agent repository',
      path:
        environment.LTHN_AGENT_REPO ||
        join(homeDir, 'Code', 'core', 'agent'),
      required: false,
      hint: 'Optional: set LTHN_AGENT_REPO when the agent repository lives elsewhere.',
    },
    {
      name: 'go-ai repository',
      path:
        environment.LTHN_AI_REPO ||
        join(homeDir, 'Code', 'core', 'go-ai'),
      required: false,
      hint: 'Optional: set LTHN_AI_REPO when go-ai lives elsewhere.',
    },
    {
      name: 'go-inference repository',
      path:
        environment.LTHN_LEM_REPO ||
        join(homeDir, 'Code', 'core', 'go-inference'),
      required: false,
      hint: 'Optional: set LTHN_LEM_REPO when go-inference lives elsewhere.',
    },
  ];

  const pathChecks = await Promise.all(
    paths.map(async (entry) => {
      const available = await pathProbe(entry.path);
      return {
        kind: 'path',
        name: entry.name,
        status: available ? 'ok' : entry.required ? 'error' : 'warning',
        detail: available ? entry.path : `Not found: ${entry.path}`,
        hint: available ? '' : entry.hint,
      };
    }),
  );

  const ports = await Promise.all(
    PORTS.map(async (entry) => {
      const result = await portProbe(entry.port);
      return {
        kind: 'port',
        name: entry.name,
        status: result.available ? 'ok' : 'warning',
        detail: result.detail,
        hint: result.available ? '' : entry.hint,
      };
    }),
  );

  const lemName = process.platform === 'win32' ? 'lem.exe' : 'lem';
  const explicitLEM = environment.LTHN_LEM_BIN || '';
  const lemCandidates = explicitLEM
    ? [explicitLEM]
    : [
        join(cwd, 'bin', lemName),
        join(cwd, 'build', platformDirectory(process.platform), 'bin', lemName),
      ];
  let readyLEM = '';
  for (const candidate of lemCandidates) {
    if (await pathProbe(candidate)) {
      readyLEM = candidate;
      break;
    }
  }
  const lemCheck = {
    kind: 'path',
    name: 'LEM sidecar',
    status: readyLEM ? 'ok' : 'warning',
    detail: readyLEM
      ? `ready at ${displayPath(cwd, readyLEM)}`
      : 'optional runtime unavailable; build it or set LTHN_LEM_BIN',
    hint: readyLEM
      ? ''
      : 'Run task build:lem, or point LTHN_LEM_BIN at a matching-platform binary.',
  };

  const checks = [...commands, ...pathChecks, lemCheck, ...ports];
  return {
    ok: checks.every(({ status }) => status !== 'error'),
    checks,
  };
}

function platformDirectory(platform) {
  if (platform === 'win32') return 'windows';
  return platform;
}

function displayPath(cwd, path) {
  const candidate = relative(cwd, path);
  if (candidate === '' || candidate.startsWith('..') || isAbsolute(candidate)) {
    return path;
  }
  return candidate.replaceAll('\\', '/');
}

export function renderDoctorReport(report) {
  const labels = {
    ok: 'OK',
    warning: 'WARN',
    error: 'FAIL',
  };
  const lines = ['Lethean Desktop development doctor', ''];
  for (const check of report.checks) {
    lines.push(`[${labels[check.status]}] ${check.name}: ${check.detail}`);
    if (check.hint) lines.push(`       ${check.hint}`);
  }

  const errors = report.checks.filter(({ status }) => status === 'error').length;
  const warnings = report.checks.filter(({ status }) => status === 'warning').length;
  lines.push('');
  if (errors) {
    lines.push(
      `Development environment needs attention: ${errors} error${errors === 1 ? '' : 's'}, ${warnings} warning${warnings === 1 ? '' : 's'}.`,
    );
  } else if (warnings) {
    lines.push(
      `Ready with ${warnings} warning${warnings === 1 ? '' : 's'}; optional or occupied resources are listed above.`,
    );
  } else {
    lines.push('Ready for development.');
  }
  return lines.join('\n');
}

export async function probeCommand({ command, args }) {
  try {
    const { stdout, stderr } = await execFile(command, args, {
      timeout: 5_000,
      maxBuffer: 1024 * 1024,
    });
    return {
      available: true,
      detail: (stdout || stderr).trim().split(/\r?\n/, 1)[0] || 'available',
    };
  } catch (error) {
    return {
      available: false,
      detail:
        error instanceof Error && error.message
          ? error.message.split(/\r?\n/, 1)[0]
          : 'command unavailable',
    };
  }
}

export async function probePath(path) {
  try {
    await access(path);
    return true;
  } catch {
    return false;
  }
}

export async function probePort(port) {
  return await new Promise((resolveProbe) => {
    const server = createServer();
    server.unref();
    server.once('error', (error) => {
      const detail =
        error && typeof error === 'object' && 'code' in error && error.code === 'EADDRINUSE'
          ? `127.0.0.1:${port} is already in use`
          : `127.0.0.1:${port} could not be checked`;
      resolveProbe({ available: false, detail });
    });
    server.listen({ host: '127.0.0.1', port }, () => {
      server.close(() => {
        resolveProbe({
          available: true,
          detail: `127.0.0.1:${port} is available`,
        });
      });
    });
  });
}

async function main() {
  const report = await inspectDevelopmentEnvironment();
  process.stdout.write(`${renderDoctorReport(report)}\n`);
  if (!report.ok) process.exitCode = 1;
}

if (process.argv[1] && resolve(process.argv[1]) === scriptPath) {
  await main();
}
