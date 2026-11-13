const HOUR_MS = 3_600_000;
const DAY_MS = 24 * HOUR_MS;
const WEEK_MS = 7 * DAY_MS;
const SUMMARY_LEAD_MS = HOUR_MS;
const PACE = 'pace';
const SUMMARY = 'summary';
const STALE = 'stale';
const STALE_MS = 24 * HOUR_MS;
const FULL = 100;

export function rulesFrom(text) {
  const parts = (text ?? '')
    .split(/[\s,]+/)
    .map((part) => part.trim().toLowerCase())
    .filter(Boolean);
  const thresholds = parts
    .filter((part) => ![PACE, SUMMARY, STALE].includes(part))
    .map(Number)
    .filter((value) => Number.isFinite(value) && value > 0 && value <= FULL)
    .sort((a, b) => a - b);
  return {
    thresholds,
    pace: parts.includes(PACE),
    summary: parts.includes(SUMMARY),
    stale: parts.includes(STALE),
  };
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

export function staleMembers(people, now, previous = {}) {
  const state = {};
  const alerts = [];
  for (const person of people) {
    if (person.is_active === false || !person.last_synced_at) continue;
    const syncedAt = Date.parse(person.last_synced_at);
    if (now - syncedAt < STALE_MS) continue;
    state[person.user_id] = person.last_synced_at;
    if (previous[person.user_id] === person.last_synced_at) continue;
    alerts.push({
      name: person.full_name || person.email,
      syncedAt,
      hours: Math.floor((now - syncedAt) / HOUR_MS),
    });
  }
  return { state, alerts };
}

const OUTPUT_WEIGHT = 5;
const TOP_PROJECTS = 3;

function weeklyLine(account, now) {
  const left = account.resetsAt - now;
  const used = Math.round(account.used);
  const parts = [`Weekly limit ${used}% used`];
  if (left > 0) {
    const days = left / DAY_MS;
    const perDay = Math.max(0, FULL - account.used) / Math.max(days, 1);
    parts.push(`budget ${perDay.toFixed(1)}%/day for ${days.toFixed(1)} days`);
    const elapsed = WEEK_MS - left;
    if (elapsed >= DAY_MS && account.used > 0) {
      const projected = (account.used / elapsed) * WEEK_MS;
      parts.push(
        projected > FULL && account.used < FULL
          ? `at this pace it runs out in ${resetsIn(((FULL - account.used) / account.used) * elapsed)}`
          : `on pace for ${Math.round(projected)}% at reset`
      );
    }
  }
  return parts.join(' · ');
}

function projectWeight(project) {
  return (
    project.input_tokens +
    project.cache_write_tokens +
    project.cache_read_tokens +
    OUTPUT_WEIGHT * project.output_tokens
  );
}

function topProjectsLine(projects) {
  const weighed = projects
    .map((project) => ({ name: project.project || 'Private', weight: projectWeight(project) }))
    .filter((project) => project.weight > 0)
    .sort((a, b) => b.weight - a.weight);
  const total = weighed.reduce((sum, project) => sum + project.weight, 0);
  if (total === 0) return null;
  const share = (weight) => `${Math.round((weight / total) * FULL)}%`;
  const parts = weighed.slice(0, TOP_PROJECTS).map((project) => `${project.name} ${share(project.weight)}`);
  const rest = weighed.slice(TOP_PROJECTS).reduce((sum, project) => sum + project.weight, 0);
  if (rest > 0) parts.push(`others ${share(rest)}`);
  return `Top projects, last 24h: ${parts.join(' · ')}`;
}

export function dailySummary(people, projects, now) {
  const accounts = accountsFrom(people);
  const several = accounts.filter((account) => account.weekly).length > 1;
  const lines = accounts.map((account) => {
    const line = account.weekly
      ? weeklyLine(account, now)
      : `${account.label.replace(/^Claude /, '')} ${Math.round(account.used)}% used`;
    return several ? `${line} (${account.members.join(', ')})` : line;
  });
  const top = topProjectsLine(projects);
  if (top) lines.push(top);
  return lines.join('\n');
}
