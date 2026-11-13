const HOUR_MS = 3_600_000;
const DAY_MS = 24 * HOUR_MS;
const WEEK_MS = 7 * DAY_MS;
const PACE = 'pace';
const FULL = 100;

export function rulesFrom(text) {
  const parts = (text ?? '')
    .split(/[\s,]+/)
    .map((part) => part.trim().toLowerCase())
    .filter(Boolean);
  const thresholds = parts
    .filter((part) => part !== PACE)
    .map(Number)
    .filter((value) => Number.isFinite(value) && value > 0 && value <= FULL)
    .sort((a, b) => a - b);
  return { thresholds, pace: parts.includes(PACE) };
}

export function accountsFrom(people) {
  const accounts = new Map();
  for (const person of people) {
    if (person.is_active === false || person.seven_day_used == null || !person.seven_day_resets_at) continue;
    const key = (person.claude_email || person.user_id).toLowerCase();
    const account = accounts.get(key) ?? {
      key,
      used: person.seven_day_used,
      resetsAt: Date.parse(person.seven_day_resets_at),
      members: [],
    };
    account.members.push(person.full_name || person.email);
    accounts.set(key, account);
  }
  return [...accounts.values()];
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
  const who = account.members.join(', ');
  const left = resetsIn(account.resetsAt - now);
  const crossed = rules.thresholds.filter((value) => account.used >= value && !sent.includes(String(value)));
  if (crossed.length > 0) {
    state.sent.push(...crossed.map(String));
    return {
      state,
      text: `Claude weekly limit at ${Math.round(account.used)}% (${who}). Resets in ${left}.`,
    };
  }
  const elapsed = 1 - (account.resetsAt - now) / WEEK_MS;
  const onPace =
    elapsed >= 1 / 7 && account.used < (rules.thresholds[0] ?? FULL) && account.used / elapsed >= FULL;
  if (rules.pace && onPace && !sent.includes(PACE)) {
    state.sent.push(PACE);
    return {
      state,
      text: `Claude weekly limit is on pace to run out before the reset: ${Math.round(account.used)}% used with ${Math.round(elapsed * FULL)}% of the week gone (${who}). Resets in ${left}.`,
    };
  }
  return { state, text: null };
}
