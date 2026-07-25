import { spawn } from 'node:child_process';
import { access } from 'node:fs/promises';

export function gitLines(repoRoot, args) {
  return new Promise((resolve, reject) => {
    const child = spawn('git', args, { cwd: repoRoot });
    let stdout = '';
    let stderr = '';
    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    child.stdout.on('data', (chunk) => { stdout += chunk; });
    child.stderr.on('data', (chunk) => { stderr += chunk; });
    child.on('error', reject);
    child.on('close', (code) => {
      if (code !== 0) {
        reject(new Error(`git ${args.join(' ')} failed: ${stderr.trim()}`));
        return;
      }
      resolve(stdout.split('\n').filter(Boolean));
    });
  });
}

export async function pathExists(path) {
  try {
    await access(path);
    return true;
  } catch {
    return false;
  }
}
