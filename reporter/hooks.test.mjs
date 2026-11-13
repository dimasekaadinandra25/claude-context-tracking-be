import assert from 'node:assert/strict';
import { test } from 'node:test';

import { dueDay, hooksFrom, kindOf, localNow, redact, requestFor, textRequestFor } from './hooks.mjs';

const png = new Uint8Array([137, 80, 78, 71]);

test('detects discord, telegram and generic urls', () => {
  assert.equal(kindOf('https://discord.com/api/webhooks/1/abc'), 'discord');
  assert.equal(kindOf('https://api.telegram.org/bot1:x/sendPhoto?chat_id=5'), 'telegram');
  assert.equal(kindOf('https://api.fonnte.com/send'), 'generic');
  assert.equal(kindOf('https://discord.com/channels/1'), 'generic');
});

test('reads a url list or a json list', () => {
  assert.deepEqual(hooksFrom(''), []);
  const list = hooksFrom('https://discord.com/api/webhooks/1/a, https://example.com/hook');
  assert.deepEqual(list.map((hook) => hook.kind), ['discord', 'generic']);
  const [wa] = hooksFrom(
    '[{"url":"https://api.fonnte.com/send","headers":{"Authorization":"t"},"fields":{"target":"0812"}}]'
  );
  assert.equal(wa.file_field, 'file');
  assert.deepEqual(wa.headers, { Authorization: 't' });
  assert.throws(() => hooksFrom('[{"kind":"discord"}]'), /needs a url/);
  assert.throws(() => hooksFrom('[{"url":"https://x.test","kind":"fax"}]'), /unknown webhook kind/);
});

test('builds the multipart body each service expects', async () => {
  const [discord] = hooksFrom('https://discord.com/api/webhooks/1/a');
  let form = requestFor(discord, png, 'r.png', 'hello').init.body;
  assert.deepEqual(JSON.parse(form.get('payload_json')), { content: 'hello' });
  assert.equal(form.get('files[0]').name, 'r.png');

  const [telegram] = hooksFrom('https://api.telegram.org/bot1:x/sendPhoto?chat_id=5');
  form = requestFor(telegram, png, 'r.png', 'hello').init.body;
  assert.equal(form.get('caption'), 'hello');
  assert.equal(form.get('photo').type, 'image/png');

  const [wa] = hooksFrom('[{"url":"https://api.fonnte.com/send","fields":{"target":"0812"}}]');
  const request = requestFor(wa, png, 'r.png', 'hello');
  assert.equal(request.init.body.get('target'), '0812');
  assert.equal(request.init.body.get('message'), 'hello');
  assert.equal((await request.init.body.get('file').arrayBuffer()).byteLength, png.length);
});

test('is due once per local day after the send time', () => {
  const at = new Date('2026-11-11T02:05:00Z');
  assert.deepEqual(localNow(at, 'Asia/Jakarta'), { day: '2026-11-11', time: '09:05' });
  assert.equal(dueDay(at, 'Asia/Jakarta', '09:00', null), '2026-11-11');
  assert.equal(dueDay(at, 'Asia/Jakarta', '09:00', '2026-11-11'), null);
  assert.equal(dueDay(at, 'Asia/Jakarta', '10:00', null), null);
});

test('never logs the secret part of a webhook url', () => {
  assert.equal(redact('https://discord.com/api/webhooks/123/SECRET'), 'https://discord.com/…');
  assert.ok(!redact('https://api.telegram.org/bot1:SECRET/sendPhoto').includes('SECRET'));
});

test('sends plain text alerts in each webhook format', async () => {
  const [discord, telegram, wa] = hooksFrom(
    '[{"url":"https://discord.com/api/webhooks/1/a"},{"url":"https://api.telegram.org/bot1:x/sendPhoto","fields":{"chat_id":"5"}},{"url":"https://api.fonnte.com/send","fields":{"target":"0812"}}]'
  );
  const d = textRequestFor(discord, 'hi');
  assert.equal(d.init.body, '{"content":"hi"}');
  assert.equal(d.init.headers['Content-Type'], 'application/json');
  const t = textRequestFor(telegram, 'hi');
  assert.equal(t.url, 'https://api.telegram.org/bot1:x/sendMessage');
  assert.equal(t.init.body.get('text'), 'hi');
  assert.equal(t.init.body.get('chat_id'), '5');
  const w = textRequestFor(wa, 'hi');
  assert.equal(w.init.body.get('message'), 'hi');
  assert.equal(w.init.body.get('target'), '0812');
});
