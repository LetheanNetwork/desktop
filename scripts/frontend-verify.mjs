import { spawn } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptPath = fileURLToPath(import.meta.url);
const frontendRoot = resolve(dirname(scriptPath), '..', 'frontend');

export const FRONTEND_VERIFICATION_STEPS = Object.freeze([
  'audit:capabilities',
  'test:contracts',
  'test:ci',
  'build',
  'verify:build',
]);

export async function runFrontendVerification(runStep = runNpmScript) {
  for (const step of FRONTEND_VERIFICATION_STEPS) {
    await runStep(step);
  }
}

function runNpmScript(step) {
  const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm';
  return new Promise((resolveStep, rejectStep) => {
    const child = spawn(npm, ['run', step], {
      cwd: frontendRoot,
      env: process.env,
      stdio: 'inherit',
    });
    child.once('error', rejectStep);
    child.once('exit', (code, signal) => {
      if (code === 0) {
        resolveStep();
        return;
      }
      const ending = signal ? `signal ${signal}` : `exit code ${code ?? 'unknown'}`;
      rejectStep(new Error(`Frontend verification step "${step}" failed with ${ending}.`));
    });
  });
}

if (process.argv[1] && resolve(process.argv[1]) === scriptPath) {
  await runFrontendVerification();
}
