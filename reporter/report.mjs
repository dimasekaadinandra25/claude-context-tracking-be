import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright';

import { dueDay, hooksFrom, localNow, redact, requestFor } from './hooks.mjs';

const env = (name, fallback) => process.env[name]?.trim() || fallback;

const API_URL = env('REPORT_API_URL', 'http://api:8080');
const DASHBOARD_URL = env('REPORT_DASHBOARD_URL', 'http://dashboard:80');
const PUBLIC_API_URL = env('REPORT_PUBLIC_API_URL', 'http://localhost:8080');
const PUBLIC_DASHBOARD_URL = env('REPORT_PUBLIC_DASHBOARD_URL', 'http://localhost:3000');
const SEND_AT = env('REPORT_TIME', '09:00');
const TIME_ZONE = env('REPORT_TZ', 'Asia/Jakarta');
const THEME = env('REPORT_THEME', 'dark');
const LOCALE = env('REPORT_LOCALE', 'en-US');
const MESSAGE = env('REPORT_MESSAGE', 'Claude usage report {date}');
const CHECK_MS = 30_000;
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
      ({ access, refresh, theme }) => {
        sessionStorage.setItem('claude_usage.access_token', access);
        localStorage.setItem('claude_usage.refresh_token', refresh);
        localStorage.setItem('theme-mode', theme);
      },
      { access: session.access_token, refresh: session.refresh_token, theme: THEME }
    );
    const page = await context.newPage();
    await page.goto(`${PUBLIC_DASHBOARD_URL}/dashboard/overview`, { waitUntil: 'networkidle' });
    const button = page.getByRole('button', { name: 'Export image' });
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

async function send(hooks, png, fileName, message) {
  let failed = 0;
  for (const hook of hooks) {
    const { url, init } = requestFor(hook, png, fileName, message);
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

async function report(hooks, day) {
  const { png, fileName } = await capture();
  log(`captured ${fileName} (${png.length} bytes)`);
  const failed = await send(hooks, png, fileName, MESSAGE.replace('{date}', day));
  if (failed === hooks.length) throw new Error('no webhook accepted the report');
}

async function main() {
  const hooks = hooksFrom(process.env.REPORT_WEBHOOKS);
  const today = () => localNow(new Date(), TIME_ZONE).day;

  if (process.argv.includes('--now')) {
    if (hooks.length === 0) throw new Error('REPORT_WEBHOOKS is empty');
    await report(hooks, today());
    return;
  }
  if (hooks.length === 0) {
    log('REPORT_WEBHOOKS is empty, nothing to send');
    setInterval(() => {}, 2 ** 31 - 1);
    return;
  }

  let lastDay = localNow(new Date(), TIME_ZONE).time >= SEND_AT ? today() : null;
  let tries = 0;
  let nextTry = 0;
  log(`sending daily at ${SEND_AT} ${TIME_ZONE} to ${hooks.length} webhook(s)`);
  setInterval(async () => {
    const day = dueDay(new Date(), TIME_ZONE, SEND_AT, lastDay);
    if (!day || Date.now() < nextTry) return;
    nextTry = Date.now() + RETRY_MS;
    try {
      await report(hooks, day);
      lastDay = day;
      tries = 0;
    } catch (error) {
      tries += 1;
      log(`report for ${day} failed (try ${tries}/${MAX_TRIES}): ${error.message}`);
      if (tries >= MAX_TRIES) {
        lastDay = day;
        tries = 0;
      }
    }
  }, CHECK_MS);
}

main().catch((error) => {
  log(error.message);
  process.exit(1);
});
