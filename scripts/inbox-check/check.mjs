#!/usr/bin/env node
/**
 * SendSMTP inbox check — own Playwright verifier.
 * stdin JSON: { marker, headless, timeout_ms, seeds:[{provider,email,password}] }
 * stdout JSON: { results:[{email,provider,placement,error?}] }
 */
import { chromium } from "playwright";
import { readFileSync } from "fs";

function readInput() {
  const raw = readFileSync(0, "utf8");
  return JSON.parse(raw || "{}");
}

function out(obj) {
  process.stdout.write(JSON.stringify(obj));
}

async function checkGmail(page, seed, marker, timeout) {
  page.setDefaultTimeout(timeout);
  await page.goto("https://accounts.google.com/signin/v2/identifier?service=mail&continue=https://mail.google.com/mail/u/0/", {
    waitUntil: "domcontentloaded",
  });

  // Email step
  const emailSel = 'input[type="email"]';
  await page.waitForSelector(emailSel, { timeout: Math.min(timeout, 45000) });
  await page.fill(emailSel, seed.email);
  await page.click("#identifierNext, button:has-text('Next'), button:has-text('Avançar')");
  await page.waitForTimeout(1200);

  // Password step (app password)
  const passSel = 'input[type="password"]';
  await page.waitForSelector(passSel, { timeout: Math.min(timeout, 45000) });
  await page.fill(passSel, seed.password);
  await page.click("#passwordNext, button:has-text('Next'), button:has-text('Avançar')");
  await page.waitForTimeout(2500);

  // Challenge / 2FA hard fail
  const url = page.url();
  if (url.includes("challenge") || url.includes("signin/rejected") || url.includes("speedbump")) {
    throw new Error("login bloqueado (2FA/challenge) — use App Password e conta sem challenge interativo");
  }

  await page.goto("https://mail.google.com/mail/u/0/#inbox", { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(2000);

  const q = encodeURIComponent(marker);
  await page.goto(`https://mail.google.com/mail/u/0/#search/${q}`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(2500);
  let html = await page.content();
  const inInbox =
    html.toLowerCase().includes(marker.toLowerCase()) &&
    !html.includes("No messages matched your search") &&
    !html.includes("Nenhuma mensagem corresponde");

  if (inInbox) {
    // Prefer spam confirmation only if also in spam? If found in general search it may be spam too.
    await page.goto(`https://mail.google.com/mail/u/0/#search/in%3Aspam+${q}`, { waitUntil: "domcontentloaded" });
    await page.waitForTimeout(2000);
    html = await page.content();
    const inSpam =
      html.toLowerCase().includes(marker.toLowerCase()) &&
      !html.includes("No messages matched your search") &&
      !html.includes("Nenhuma mensagem corresponde");
    if (inSpam) return "spam";
    return "inbox";
  }

  await page.goto(`https://mail.google.com/mail/u/0/#search/in%3Aspam+${q}`, { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(2500);
  html = await page.content();
  const inSpam =
    html.toLowerCase().includes(marker.toLowerCase()) &&
    !html.includes("No messages matched your search") &&
    !html.includes("Nenhuma mensagem corresponde");
  if (inSpam) return "spam";
  return "missing";
}

async function checkOutlook(page, seed, marker, timeout) {
  page.setDefaultTimeout(timeout);
  await page.goto("https://outlook.live.com/mail/0/", { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(1500);

  // Redirect to login
  if (page.url().includes("login") || (await page.locator('input[type="email"]').count()) > 0) {
    await page.fill('input[type="email"]', seed.email);
    await page.click('input[type="submit"], button[type="submit"]');
    await page.waitForTimeout(1200);
    await page.fill('input[type="password"]', seed.password);
    await page.click('input[type="submit"], button[type="submit"]');
    await page.waitForTimeout(2500);
    // Stay signed in
    const yes = page.locator('#idSIButton9, input[value="Yes"], input[value="Sim"]');
    if ((await yes.count()) > 0) {
      await yes.first().click().catch(() => {});
      await page.waitForTimeout(1500);
    }
  }

  await page.goto("https://outlook.live.com/mail/0/inbox", { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(2000);

  // Search box
  const search = page.locator('input[aria-label*="Search"], input[placeholder*="Search"], input[type="search"]').first();
  if ((await search.count()) === 0) {
    throw new Error("outlook search box not found");
  }
  await search.fill(marker);
  await page.keyboard.press("Enter");
  await page.waitForTimeout(2500);
  let body = (await page.content()).toLowerCase();
  if (body.includes(marker.toLowerCase()) && !body.includes("we didn't find anything")) {
    return "inbox";
  }

  await page.goto("https://outlook.live.com/mail/0/junkemail", { waitUntil: "domcontentloaded" });
  await page.waitForTimeout(2000);
  if ((await search.count()) > 0) {
    await search.fill(marker);
    await page.keyboard.press("Enter");
    await page.waitForTimeout(2500);
  }
  body = (await page.content()).toLowerCase();
  if (body.includes(marker.toLowerCase())) return "spam";
  return "missing";
}

async function main() {
  const input = readInput();
  const marker = String(input.marker || "").trim();
  const seeds = Array.isArray(input.seeds) ? input.seeds : [];
  const headless = input.headless !== false;
  const timeout = Number(input.timeout_ms) || 180000;

  if (!marker) {
    out({ error: "marker vazio" });
    process.exit(1);
  }
  if (!seeds.length) {
    out({ error: "nenhuma seed configurada" });
    process.exit(1);
  }

  const browser = await chromium.launch({ headless });
  const results = [];
  try {
    for (const seed of seeds) {
      const provider = String(seed.provider || "gmail").toLowerCase();
      const context = await browser.newContext({
        viewport: { width: 1280, height: 900 },
        locale: "en-US",
      });
      const page = await context.newPage();
      try {
        let placement = "missing";
        if (provider === "outlook" || provider === "hotmail") {
          placement = await checkOutlook(page, seed, marker, timeout);
        } else {
          placement = await checkGmail(page, seed, marker, timeout);
        }
        results.push({
          email: seed.email,
          provider,
          placement,
        });
      } catch (e) {
        results.push({
          email: seed.email,
          provider,
          placement: "error",
          error: String(e?.message || e),
        });
      } finally {
        await context.close().catch(() => {});
      }
    }
  } finally {
    await browser.close().catch(() => {});
  }

  out({ results });
}

main().catch((e) => {
  out({ error: String(e?.message || e), results: [] });
  process.exit(1);
});
