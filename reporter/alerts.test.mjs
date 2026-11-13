import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  accountsFrom,
  anyRule,
  check,
  dailySummary,
  lastWeekUsed,
  projectShares,
  rulesFrom,
  runawaySessions,
  staleMembers,
  summarySent,
} from './alerts.mjs';

const HOUR = 3_600_000;
const DAY = 24 * HOUR;
const now = Date.parse('2026-11-13T00:00:00Z');
const rules = rulesFrom('80, 90, pace, summary');

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
  assert.deepEqual(rulesFrom('90,80,pace,abc,0,120,summary,stale,runaway,project'), {
    thresholds: [80, 90],
    pace: true,
    summary: true,
    stale: true,
    runaway: true,
    project: true,
  });
  const none = rulesFrom('');
  assert.equal(anyRule(none), false);
  assert.equal(anyRule(rulesFrom('stale')), true);
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

test('single-model limits such as Fable alert on their own, without pace or summary', () => {
  const resets = new Date(now + 2 * DAY).toISOString();
  const accounts = accountsFrom([
    person({ seven_day_used: 30, model_limits: [{ name: 'Fable', used: 85, resets_at: resets }] }),
  ]);
  assert.deepEqual(accounts.map((a) => a.key), ['team@x.test', 'team@x.test|fable']);
  const fable = check(accounts[1], now, rules, undefined);
  assert.match(fable.text, /^Claude Fable weekly limit at 85% \(Dummy\)\. Resets in 2d 0h\.$/);
  assert.equal(fable.summary, false);
  assert.equal(check(accounts[1], now, rules, fable.state).text, null);
  const [, quiet] = accountsFrom([person({ model_limits: [{ name: 'Fable', used: 10, resets_at: resets }] })]);
  assert.equal(check(quiet, now + 6.5 * DAY, rules, undefined).text, null);
});

test('weekly summary is due once in the last hour before the reset', () => {
  const [account] = accountsFrom([person({ seven_day_used: 40 })]);
  const early = check(account, account.resetsAt - 2 * HOUR, rules, undefined);
  assert.equal(early.summary, false);
  const due = check(account, account.resetsAt - 30 * 60_000, rules, early.state);
  assert.equal(due.summary, true);
  assert.equal(check(account, account.resetsAt - 25 * 60_000, rules, due.state).summary, true);
  assert.equal(check(account, account.resetsAt - 25 * 60_000, rules, summarySent(due.state)).summary, false);
  assert.equal(check(account, account.resetsAt - 30 * 60_000, rulesFrom('80'), undefined).summary, false);
});

test('a member whose app stopped syncing for 24h is reported once until it syncs again', () => {
  const at = (hours) => new Date(now - hours * HOUR).toISOString();
  const people = [
    person({ user_id: 'a', full_name: 'Dummy 1', last_synced_at: at(30) }),
    person({ user_id: 'b', full_name: 'Dummy 2', last_synced_at: at(2) }),
    person({ user_id: 'c', full_name: 'Gone', is_active: false, last_synced_at: at(90) }),
    person({ user_id: 'd', full_name: 'New', last_synced_at: null }),
  ];
  const first = staleMembers(people, now, undefined);
  assert.deepEqual(first.alerts.map((a) => [a.name, a.hours]), [['Dummy 1', 30]]);
  assert.deepEqual(staleMembers(people, now, first.state).alerts, []);
  people[0].last_synced_at = at(1);
  const back = staleMembers(people, now, first.state);
  assert.deepEqual(back.state, {});
  people[0].last_synced_at = at(26);
  assert.equal(staleMembers(people, now, back.state).alerts.length, 1);
});

test('daily message lists the weekly budget, model limits and top projects', () => {
  const project = (name, input, output) => ({
    project: name,
    input_tokens: input,
    cache_write_tokens: 0,
    cache_read_tokens: 0,
    output_tokens: output,
  });
  const text = dailySummary(
    [person({ model_limits: [{ name: 'Fable', used: 10, resets_at: new Date(now + 3 * DAY).toISOString() }] })],
    [project('api', 600, 0), project('web', 100, 20), project('a', 50, 0), project('b', 50, 0), project('', 0, 0)],
    now
  );
  assert.equal(
    text,
    [
      'Weekly limit 50% used · budget 16.7%/day for 3.0 days · on pace for 88% at reset',
      'Fable weekly limit 10% used',
      'Top projects, last 24h: api 67% · web 22% · a 6% · others 6%',
    ].join('\n')
  );
  const fast = dailySummary([person({ seven_day_used: 80 })], [], now);
  assert.match(fast, /^Weekly limit 80% used · budget 6\.7%\/day for 3\.0 days · at this pace it runs out in 1d 0h$/);
});

test('daily message compares with the same day last week and links the dashboard', () => {
  const people = [person({ user_id: 'u1', claude_email: 'Team@x.test' })];
  const daily = [
    { user_id: 'u1', day: '2026-11-06', seven_day_used: 45 },
    { user_id: 'u1', day: '2026-11-05', seven_day_used: 30 },
  ];
  const lastWeek = lastWeekUsed(people, daily, '2026-11-06', 'Fri 6');
  assert.deepEqual([...lastWeek.entries()], [['team@x.test', { label: 'Fri 6', used: 45 }]]);
  const text = dailySummary(people, [], now, lastWeek, 'https://usage.example.com/dashboard/overview');
  assert.equal(
    text,
    'Weekly limit 50% used · budget 16.7%/day for 3.0 days · on pace for 88% at reset · last week by Fri 6: 45%\nhttps://usage.example.com/dashboard/overview'
  );
});

test('a runaway session is reported once per hour and forgotten after a day', () => {
  const hour = new Date(now - HOUR).toISOString();
  const hot = [{ session_id: 's1', full_name: 'Dummy 1', title: 'Fix login', project: 'api', hour, share: 6.24 }];
  const first = runawaySessions(hot, now, undefined);
  assert.deepEqual(first.alerts, [
    `Runaway session: Dummy 1's "Fix login in api" used 6.2% of the weekly limit within one hour.`,
  ]);
  assert.deepEqual(runawaySessions(hot, now, first.state).alerts, []);
  assert.deepEqual(runawaySessions([], now + 2 * DAY, first.state).state, {});
});

test('a project passing the share of the week is reported once per window, not on the first day', () => {
  const project = (name, input) => ({ project: name, input_tokens: input, cache_write_tokens: 0, cache_read_tokens: 0, output_tokens: 0 });
  const projects = [project('api', 450), project('web', 400), project('', 150)];
  const window = now + 3 * DAY;
  const first = projectShares(projects, 40, window, now, undefined);
  assert.deepEqual(first.alerts, [
    `Project api has used 45% of the team's Claude tokens this week (alert at 40%).`,
    `Project web has used 40% of the team's Claude tokens this week (alert at 40%).`,
  ]);
  assert.deepEqual(projectShares(projects, 40, window, now, first.state).alerts, []);
  assert.equal(projectShares(projects, 40, now + 5 * DAY, now, first.state).alerts.length, 2);
  assert.deepEqual(projectShares(projects, 40, now + 6.5 * DAY, now, undefined).alerts, []);
});

test('daily message shows the 5-hour window with each member share and who is active now', () => {
  const people = [
    person({ user_id: 'a', full_name: 'Dummy 1', five_hour_used: 34, five_hour_share: 22.4, last_request_at: new Date(now - 60_000).toISOString() }),
    person({ user_id: 'b', full_name: 'Dummy 2', five_hour_used: 34, five_hour_share: 11.6, last_request_at: new Date(now - HOUR).toISOString() }),
    person({ user_id: 'c', full_name: 'Dummy 3', five_hour_used: 34, five_hour_share: 0, last_request_at: null }),
  ];
  const lines = dailySummary(people, [], now).split(String.fromCharCode(10));
  assert.equal(lines[1], '5-hour window 34% used: Dummy 1 22% (active now), Dummy 2 12%');
});
