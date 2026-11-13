const HOUR_MS = 3_600_000;
const DAY_MS = 24 * HOUR_MS;
const WEEK_MS = 7 * DAY_MS;
const SUMMARY_LEAD_MS = HOUR_MS;
const PACE = 'pace';
const SUMMARY = 'summary';
const FULL = 100;

export function rulesFrom(text) {
  const parts = (text ?? '')
    .split(/[\s,]+/)
    .map((part) => part.trim().toLowerCase())
    .filter(Boolean);
  const thresholds = parts
    .filter((part) => part !== PACE && part !== SUMMARY)
    .map(Number)
    .filter((value) => Number.isFinite(value) && value > 0 && value <= FULL)
    .sort((a, b) => a - b);
  return { thresholds, pace: parts.includes(PACE), summary: parts.includes(SUMMARY) };
}

export function accountsFrom(people) {
  const accounts = new Map();
  for (const person of people) {
    if (person.is_active === false || person.seven_day_used == null || !person.seven_day_resets_at) continue;
    const key = (person.claude_email || person.user_id).toLowerCase();
    const account = accounts.get(key) ?? {
      key,
      label: 'Claude weekly limit',
      weekly: true,
      used: person.seven_day_used,
      resetsAt: Date.parse(person.seven_day_resets_at),
      models: person.model_limits ?? [],
      members: [],
    };
    account.members.push(person.full_name || person.email);
    accounts.set(key, account);
  }
  return [...accounts.values()].flatMap(({ models, ...account }) => [
    account,
    ...models.map((model) => ({
      key: `${account.key}|${model.name.toLowerCase()}`,
      label: `Claude ${model.name} weekly limit`,
      weekly: false,
      used: model.used,
      resetsAt: Date.parse(model.resets_at),
      members: account.members,
    })),
  ]);
}

function sameWindow(a, b) {
  return Math.abs(a - b) < HOUR_MS;
}

function resetsIn(ms) {
  const hours = Math.max(0, Math.round(ms / HOUR_MS));
  const days = Math.floor(hours / 24);
  return days > 0 ? `${days}d ${hours % 24}h` : `${hours}h`;
}

export function check(account, now, rules, previous) {
  const sent = previous && sameWindow(previous.resetsAt, account.resetsAt) ? previous.sent : [];
  const state = { resetsAt: account.resetsAt, sent: [...sent] };
  const left = account.resetsAt - now;
  const summary =
    rules.summary && account.weekly && left > 0 && left <= SUMMARY_LEAD_MS && !sent.includes(SUMMARY);
  const who = account.members.join(', ');
  const crossed = rules.thresholds.filter((value) => account.used >= value && !sent.includes(String(value)));
  if (crossed.length > 0) {
    state.sent.push(...crossed.map(String));
    return {
      state,
      summary,
      text: `${account.label} at ${Math.round(account.used)}% (${who}). Resets in ${resetsIn(left)}.`,
    };
  }
  const elapsed = 1 - left / WEEK_MS;
  const onPace =
    elapsed >= 1 / 7 && account.used < (rules.thresholds[0] ?? FULL) && account.used / elapsed >= FULL;
  if (rules.pace && account.weekly && onPace && !sent.includes(PACE)) {
    state.sent.push(PACE);
    return {
      state,
      summary,
      text: `${account.label} is on pace to run out before the reset: ${Math.round(account.used)}% used with ${Math.round(elapsed * FULL)}% of the week gone (${who}). Resets in ${resetsIn(left)}.`,
    };
  }
  return { state, summary, text: null };
}

export function summarySent(state) {
  return { ...state, sent: [...state.sent, SUMMARY] };
}
