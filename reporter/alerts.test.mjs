import assert from 'node:assert/strict';
import { test } from 'node:test';

import { accountsFrom, check, rulesFrom } from './alerts.mjs';

const HOUR = 3_600_000;
const DAY = 24 * HOUR;
const now = Date.parse('2026-11-13T00:00:00Z');
const rules = rulesFrom('80, 90, pace');

const person = (fields) => ({
  user_id: 'u',
  full_name: 'Dummy',
  is_active: true,
  claude_email: 'Team@x.test',
  seven_day_used: 50,
  seven_day_resets_at: new Date(now + 3 * DAY).toISOString(),
  ...fields,
});

test('reads thresholds and pace', () => {
  assert.deepEqual(rulesFrom('90,80,pace,abc,0,120'), { thresholds: [80, 90], pace: true });
  assert.deepEqual(rulesFrom(''), { thresholds: [], pace: false });
});

test('groups members of one claude account and skips inactive or unknown ones', () => {
  const accounts = accountsFrom([
    person({ full_name: 'Dummy 1' }),
    person({ full_name: 'Dummy 2', claude_email: 'team@x.test' }),
    person({ full_name: 'Gone', is_active: false }),
    person({ user_id: 'solo', claude_email: null, seven_day_used: null }),
  ]);
  assert.equal(accounts.length, 1);
  assert.deepEqual(accounts[0].members, ['Dummy 1', 'Dummy 2']);
});

test('alerts once per threshold per window and again after the reset', () => {
  const [account] = accountsFrom([person({ seven_day_used: 92 })]);
  const first = check(account, now, rules, undefined);
  assert.match(first.text, /at 92% \(Dummy\)\. Resets in 3d 0h/);
  assert.deepEqual(first.state.sent, ['80', '90']);
  assert.equal(check(account, now, rules, first.state).text, null);
  const shifted = { ...account, resetsAt: account.resetsAt + 30 * 60_000 };
  assert.equal(check(shifted, now, rules, first.state).text, null);
  const next = { ...account, resetsAt: account.resetsAt + 7 * DAY };
  assert.match(check(next, now, rules, first.state).text, /at 92%/);
});

test('warns when the week is on pace to run out', () => {
  const [account] = accountsFrom([person({ seven_day_used: 70 })]);
  const result = check(account, now, rules, undefined);
  assert.match(result.text, /on pace to run out before the reset: 70% used with 57% of the week gone/);
  assert.equal(check(account, now, rules, result.state).text, null);
  const [early] = accountsFrom([person({ seven_day_used: 20, seven_day_resets_at: new Date(now + 6.5 * DAY).toISOString() })]);
  assert.equal(check(early, now, rules, undefined).text, null);
  const [slow] = accountsFrom([person({ seven_day_used: 40 })]);
  assert.equal(check(slow, now, rules, undefined).text, null);
});
