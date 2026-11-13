import { readFile, writeFile } from 'node:fs/promises';
import { chromium } from 'playwright';

import { accountsFrom, check, rulesFrom, summarySent } from './alerts.mjs';
import { dueDay, hooksFrom, localNow, redact, requestFor, textRequestFor } from './hooks.mjs';

const env = (name, fallback) => process.env[name]?.trim() || fallback;

const API_URL = env('REPORT_API_URL', 'http://api:8080');
const DASHBOARD_URL = env('REPORT_DASHBOARD_URL', 'http://dashboard:80');
const PUBLIC_API_URL = env('REPORT_PUBLIC_API_URL', 'http://localhost:8080');
const PUBLIC_DASHBOARD_URL = env('REPORT_PUBLIC_DASHBOARD_URL', 'http://localhost:3000');
const SEND_AT = env('REPORT_TIME', '09:00');
const TIME_ZONE = env('REPORT_TZ', 'Asia/Jakarta');
const THEME = env('REPORT_THEME', 'dark');
const LOCALE = env('REPORT_LOCALE', 'en-US');
const LANGUAGE = env('REPORT_LANG', 'en');
const MESSAGE = env('REPORT_MESSAGE', 'Claude usage report {date}');
const STATE_FILE = env('REPORT_STATE_FILE', '/state/reporter.json');
const ALERTS = process.env.REPORT_ALERTS ?? '80,90,pace,summary';
const CHECK_MS = 30_000;
const ALERT_CHECK_MS = 5 * 60_000;
const DAY_MS = 86_400_000;
const RETRY_MS = 10 * 60_000;
const MAX_TRIES = 3;
const LOAD_TIMEOUT_MS = 60_000;
const EXPORT_TIMEOUT_MS = 120_000;

const log = (...parts) => console.log(new Date().toISOString(), ...parts);

async function signIn() {
  const res = await fetch(`${API_URL}/core/v1/auth/signin`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: env('REPORT_EMAIL', ''), password: env('REPORT_PASSWORD', '') }),
  });
  if (!res.ok) throw new Error(`sign in failed with ${res.status}`);
  return (await res.json()).data;
}

function internal(url) {
  return url.replace(PUBLIC_DASHBOARD_URL, DASHBOARD_URL).replace(PUBLIC_API_URL, API_URL);
}

async function capture() {
  const session = await signIn();
  const browser = await chromium.launch();
  try {
    const context = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      colorScheme: THEME,
      timezoneId: TIME_ZONE,
      locale: LOCALE,
      acceptDownloads: true,
    });
    const origins = [PUBLIC_DASHBOARD_URL, PUBLIC_API_URL];
    await context.route(
      (url) => origins.some((origin) => url.href.startsWith(origin)),
      async (route) => route.fulfill({ response: await route.fetch({ url: internal(route.request().url()) }) })
    );
    await context.addInitScript(
      ({ access, refresh, theme, language }) => {
        sessionStorage.setItem('claude_usage.access_token', access);
        localStorage.setItem('claude_usage.refresh_token', refresh);
        localStorage.setItem('theme-mode', theme);
        localStorage.setItem('i18nextLng', language);
      },
      { access: session.access_token, refresh: session.refresh_token, theme: THEME, language: LANGUAGE }
    );
    const page = await context.newPage();
    await page.goto(`${PUBLIC_DASHBOARD_URL}/dashboard/overview`, { waitUntil: 'networkidle' });
    const button = page.getByTestId('export-image');
    await button.waitFor({ timeout: LOAD_TIMEOUT_MS });
    const [download] = await Promise.all([
      page.waitForEvent('download', { timeout: EXPORT_TIMEOUT_MS }),
      button.click(),
    ]);
    return { png: await readFile(await download.path()), fileName: download.suggestedFilename() };
  } finally {
    await browser.close();
  }
}

async function loadState() {
  try {
    return JSON.parse(await readFile(STATE_FILE, 'utf8'));
  } catch {
    return {};
  }
}

async function saveState(state) {
  try {
    await writeFile(STATE_FILE, JSON.stringify(state));
  } catch (error) {
    log(`could not save ${STATE_FILE}: ${error.message}`);
  }
}

async function send(hooks, build) {
  let failed = 0;
  for (const hook of hooks) {
    const { url, init } = build(hook);
    try {
      const res = await fetch(url, init);
      if (res.ok) {
        log(`sent to ${redact(url)} (${hook.kind})`);
      } else {
        failed += 1;
        log(`${redact(url)} answered ${res.status}: ${(await res.text()).slice(0, 200)}`);
      }
    } catch (error) {
      failed += 1;
      log(`${redact(url)} failed: ${error.message}`);
    }
  }
  return failed;
}

async function report(hooks, day, message = MESSAGE.replace('{date}', day)) {
  const { png, fileName } = await capture();
  log(`captured ${fileName} (${png.length} bytes)`);
  const failed = await send(hooks, (hook) => requestFor(hook, png, fileName, message));
  if (failed === hooks.length) throw new Error('no webhook accepted the report');
}

const notify = (hooks, text) => send(hooks, (hook) => textRequestFor(hook, text));

async function people() {
  const session = await signIn();
  const to = new Date();
  const params = new URLSearchParams({
    from: new Date(to.getTime() - DAY_MS).toISOString(),
    to: to.toISOString(),
    tz: TIME_ZONE,
  });
  const res = await fetch(`${API_URL}/usage/v1/overview?${params}`, {
    headers: { Authorization: `Bearer ${session.access_token}` },
  });
  if (!res.ok) throw new Error(`overview failed with ${res.status}`);
  return (await res.json()).data.people;
}

async function checkAlerts(hooks, rules, state) {
  const now = Date.now();
  const alerts = { ...state.alerts };
  for (const account of accountsFrom(await people())) {
    const { state: next, text, summary } = check(account, now, rules, alerts[account.key]);
    alerts[account.key] = next;
    if (text) {
      log(`alert: ${text}`);
      await notify(hooks, text);
    }
    if (summary) {
      const ends = localNow(new Date(account.resetsAt), TIME_ZONE);
      const message = `Claude weekly summary: the 7-day window ends ${ends.day} ${ends.time} (${account.members.join(', ')})`;
      try {
        await report(hooks, ends.day, message);
        alerts[account.key] = summarySent(next);
      } catch (error) {
        log(`weekly summary failed, retrying on the next check: ${error.message}`);
      }
    }
  }
  state.alerts = alerts;
  await saveState(state);
}

async function main() {
  const hooks = hooksFrom(process.env.REPORT_WEBHOOKS);
  const today = () => localNow(new Date(), TIME_ZONE).day;

  const rules = rulesFrom(ALERTS);
  const state = await loadState();

  if (process.argv.includes('--now')) {
    if (hooks.length === 0) throw new Error('REPORT_WEBHOOKS is empty');
    await report(hooks, today());
    return;
  }
  if (process.argv.includes('--alerts')) {
    if (hooks.length === 0) throw new Error('REPORT_WEBHOOKS is empty');
    await checkAlerts(hooks, rules, state);
    return;
  }
  if (hooks.length === 0) {
    log('REPORT_WEBHOOKS is empty, nothing to send');
    setInterval(() => {}, 2 ** 31 - 1);
    return;
  }

  if (!state.lastDay && localNow(new Date(), TIME_ZONE).time >= SEND_AT) {
    state.lastDay = today();
    await saveState(state);
  }
  let tries = 0;
  let nextTry = 0;
  let busy = false;
  log(`sending daily at ${SEND_AT} ${TIME_ZONE} to ${hooks.length} webhook(s), last sent ${state.lastDay ?? 'never'}`);
  setInterval(async () => {
    const day = dueDay(new Date(), TIME_ZONE, SEND_AT, state.lastDay);
    if (!day || busy || Date.now() < nextTry) return;
    busy = true;
    nextTry = Date.now() + RETRY_MS;
    try {
      await report(hooks, day);
      state.lastDay = day;
      tries = 0;
      await saveState(state);
    } catch (error) {
      tries += 1;
      log(`report for ${day} failed (try ${tries}/${MAX_TRIES}): ${error.message}`);
      if (tries >= MAX_TRIES) {
        state.lastDay = day;
        tries = 0;
        await saveState(state);
        await notify(hooks, `Claude usage report for ${day} failed after ${MAX_TRIES} tries: ${error.message}`);
      }
    } finally {
      busy = false;
    }
  }, CHECK_MS);

  if (rules.thresholds.length === 0 && !rules.pace) return;
  log(`checking limits every ${ALERT_CHECK_MS / 60_000} min for ${ALERTS}`);
  const alertTick = () =>
    checkAlerts(hooks, rules, state).catch((error) => log(`limit check failed: ${error.message}`));
  alertTick();
  setInterval(alertTick, ALERT_CHECK_MS);
}

main().catch((error) => {
  log(error.message);
  process.exit(1);
});
