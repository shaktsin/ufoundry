import { chromium } from 'playwright-core';
const b = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium-1194/chrome-linux/chrome' });
const scheme = process.env.SCHEME === 'light' ? 'light' : 'dark';
const p = await b.newPage({ viewport: { width: 1440, height: 900 }, colorScheme: scheme });
const errors = [];
p.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
p.on('pageerror', (e) => errors.push(String(e)));
let failed = 0;
const step = async (name, fn) => {
  try { await fn(); console.log('ok  ', name); }
  catch (e) { failed++; console.log('FAIL', name, e.message.split('\n')[0]); await p.screenshot({ path: `${scheme}-routefail-${name.replace(/\W+/g, '_')}.png` }); }
};

await p.goto('http://127.0.0.1:5173/');
await p.getByRole('button', { name: /demo/ }).first().waitFor({ timeout: 10000 });

await step('start from a clean slate: wake every resting key', async () => {
  await p.locator('aside').getByRole('button', { name: 'Settings' }).click();
  await p.getByRole('button', { name: 'Models', exact: true }).click();
  for (let i = 0; i < 8 && (await p.getByRole('button', { name: 'Try it now' }).count()); i++) {
    await p.getByRole('button', { name: 'Try it now' }).first().click();
    await p.waitForTimeout(400);
  }
  await p.locator('aside nav').getByRole('button', { name: 'Chat' }).click();
  await p.waitForTimeout(500);
});

await step('composer shows the route and its fallbacks', async () => {
  const chip = p.locator('div[aria-label="Message composer"] span[title*="Runs on"]').first();
  await chip.waitFor({ timeout: 10000 });
  const text = (await chip.textContent())?.trim();
  const title = await chip.getAttribute('title');
  console.log('     chip:', text, '| title:', title.replace(/\n/g, ' ~ '));
  if (!/\+\d/.test(text)) throw new Error('no fallback count in the chip: ' + text);
});
await p.screenshot({ path: scheme + '-route-01-chip.png' });

await step('a rate limit moves the turn to the other key and says so', async () => {
  await p.getByPlaceholder(/Ask about demo/).fill('hello there friend');
  await p.keyboard.press('Enter');
  await p.getByText('Hello from the fake model.').first().waitFor({ timeout: 20000 });
  await p.getByText(/switched key after a rate limit/).first().waitFor({ timeout: 10000 });
  // Same model, other key: nothing changes for the reader, so the transcript
  // stays quiet and only the footer mentions it.
  if (await p.getByText(/^Switched to/).count()) throw new Error('key swap should not add a transcript note');
});
await p.screenshot({ path: scheme + '-route-02-switch.png' });

await step('changing model, not just key, is said in the transcript', async () => {
  await p.getByPlaceholder(/Ask about demo/).fill('please drop down a model');
  await p.keyboard.press('Enter');
  await p.getByText(/Switched to local-qwen after a rate limit on local-llama/).first().waitFor({ timeout: 20000 });
  await p.getByText(/switched from local-llama after a rate limit/).first().waitFor({ timeout: 10000 });
});
await p.screenshot({ path: scheme + '-route-04-modelswitch.png' });

await step('the chip now routes around the cooling key', async () => {
  const chip = p.locator('div[aria-label="Message composer"] span[title*="Runs on"]').first();
  await p.waitForTimeout(600);
  const title = await chip.getAttribute('title');
  console.log('     title:', title.replace(/\n/g, ' ~ '));
  if (!/backup/.test(title)) throw new Error('still on the rate-limited key: ' + title);
});

await step('settings lists the resting key and can wake it', async () => {
  await p.locator('aside').getByRole('button', { name: 'Settings' }).click();
  await p.getByRole('button', { name: 'Models', exact: true }).click();
  await p.getByText('Resting').first().waitFor({ timeout: 5000 });
  await p.getByText(/after a rate limit/).first().waitFor({ timeout: 5000 });
  await p.screenshot({ path: scheme + '-route-03-resting.png' });
  for (let i = 0; i < 6 && (await p.getByRole('button', { name: 'Try it now' }).count()); i++) {
    await p.getByRole('button', { name: 'Try it now' }).first().click();
    await p.waitForTimeout(500);
  }
  if (await p.getByText('Resting').count()) throw new Error('cooldowns not cleared');
});

console.log('console errors:', errors.length ? errors.slice(0, 5) : 'none');
console.log(failed ? `${failed} step(s) failed` : 'all steps passed');
await b.close();
process.exit(failed ? 1 : 0);
