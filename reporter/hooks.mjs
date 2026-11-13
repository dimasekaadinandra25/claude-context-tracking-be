const DISCORD_HOSTS = new Set(['discord.com', 'discordapp.com', 'canary.discord.com', 'ptb.discord.com']);
const TELEGRAM_HOST = 'api.telegram.org';

const DEFAULTS = {
  discord: { file_field: 'files[0]', message_field: 'content' },
  telegram: { file_field: 'photo', message_field: 'caption' },
  generic: { file_field: 'file', message_field: 'message' },
};

export function kindOf(url) {
  const { hostname, pathname } = new URL(url);
  if (DISCORD_HOSTS.has(hostname) && pathname.startsWith('/api/webhooks/')) return 'discord';
  if (hostname === TELEGRAM_HOST) return 'telegram';
  return 'generic';
}

function normalize(hook) {
  if (!hook || typeof hook.url !== 'string' || !hook.url) {
    throw new Error('every webhook needs a url');
  }
  const kind = hook.kind ?? kindOf(hook.url);
  if (!DEFAULTS[kind]) throw new Error(`unknown webhook kind: ${kind}`);
  return { headers: {}, fields: {}, ...DEFAULTS[kind], ...hook, kind };
}

export function hooksFrom(text) {
  const value = (text ?? '').trim();
  if (!value) return [];
  if (value.startsWith('[')) return JSON.parse(value).map(normalize);
  return value
    .split(/[\s,]+/)
    .filter(Boolean)
    .map((url) => normalize({ url }));
}

export function requestFor(hook, png, fileName, message) {
  const form = new FormData();
  if (hook.kind === 'discord') {
    form.append('payload_json', JSON.stringify({ [hook.message_field]: message }));
  } else {
    form.append(hook.message_field, message);
  }
  for (const [name, value] of Object.entries(hook.fields)) {
    form.append(name, String(value));
  }
  form.append(hook.file_field, new Blob([png], { type: 'image/png' }), fileName);
  return { url: hook.url, init: { method: 'POST', headers: hook.headers, body: form } };
}

export function textRequestFor(hook, text) {
  if (hook.kind === 'discord') {
    return {
      url: hook.url,
      init: {
        method: 'POST',
        headers: { ...hook.headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({ [hook.message_field]: text }),
      },
    };
  }
  const form = new FormData();
  for (const [name, value] of Object.entries(hook.fields)) {
    form.append(name, String(value));
  }
  if (hook.kind === 'telegram') {
    form.append('text', text);
    return {
      url: hook.url.replace(/\/send\w+/, '/sendMessage'),
      init: { method: 'POST', headers: hook.headers, body: form },
    };
  }
  form.append(hook.message_field, text);
  return { url: hook.url, init: { method: 'POST', headers: hook.headers, body: form } };
}

export function localNow(date, timeZone) {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat('en-CA', {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    })
      .formatToParts(date)
      .map(({ type, value }) => [type, value])
  );
  return { day: `${parts.year}-${parts.month}-${parts.day}`, time: `${parts.hour}:${parts.minute}` };
}

export function dueDay(date, timeZone, sendAt, lastDay) {
  const { day, time } = localNow(date, timeZone);
  return time >= sendAt && day !== lastDay ? day : null;
}

export function redact(url) {
  return `${new URL(url).origin}/…`;
}
