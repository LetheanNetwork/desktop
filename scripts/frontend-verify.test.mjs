import assert from 'node:assert/strict';
import test from 'node:test';
import {
  FRONTEND_VERIFICATION_STEPS,
  runFrontendVerification,
} from './frontend-verify.mjs';

test('runs every frontend confidence check once in dependency order', async () => {
  const calls = [];

  await runFrontendVerification(async (step) => {
    calls.push(step);
  });

  assert.deepEqual(calls, [
    'audit:capabilities',
    'test:contracts',
    'test:ci',
    'build',
    'verify:build',
  ]);
  assert.deepEqual(FRONTEND_VERIFICATION_STEPS, calls);
});

test('stops at the first failed frontend confidence check', async () => {
  const calls = [];

  await assert.rejects(
    runFrontendVerification(async (step) => {
      calls.push(step);
      if (step === 'test:ci') throw new Error('Angular test failure');
    }),
    /Angular test failure/,
  );

  assert.deepEqual(calls, [
    'audit:capabilities',
    'test:contracts',
    'test:ci',
  ]);
});
