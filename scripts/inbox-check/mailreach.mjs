#!/usr/bin/env node
/**
 * Playwright fallback for MailReach free spam test create.
 * Input (stdin JSON): { action: "create", headless?: bool, timeout_ms?: number }
 * Output (stdout JSON): MailReach test object (public_id, public_full_id, results, ...)
 */
import { chromium } from "playwright";

const input = JSON.parse(await readStdin());
const action = input.action || "create";
const headless = input.headless !== false;
const timeout = Number(input.timeout_ms) || 180_000;

if (action !== "create") {
  fail(`unsupported action: ${action}`);
}

const browser = await chromium.launch({ headless });
const context = await browser.newContext({
  userAgent:
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
  locale: "en-US",
});
const page = await context.newPage();
page.setDefaultTimeout(timeout);

try {
  let created = null;
  page.on("response", async (res) => {
    try {
      const url = res.url();
      if (!url.includes("spamchecker-api.mailreach.co/api/v1/tests")) return;
      if (res.request().method() !== "POST") return;
      if (!res.ok()) return;
      const data = await res.json();
      if (data?.public_id && data?.public_full_id) created = data;
    } catch {
      /* ignore parse races */
    }
  });

  await page.goto("https://www.mailreach.co/email-spam-test", {
    waitUntil: "domcontentloaded",
    timeout,
  });

  // Cookie banners occasionally block SPA boot.
  for (const label of [/accept/i, /agree/i, /aceitar/i, /allow all/i]) {
    const btn = page.getByRole("button", { name: label }).first();
    if (await btn.isVisible({ timeout: 1500 }).catch(() => false)) {
      await btn.click().catch(() => {});
      break;
    }
  }

  // Wait for SPA create, or create ourselves from the page context (same-origin cookies).
  const deadline = Date.now() + Math.min(timeout, 60_000);
  while (!created && Date.now() < deadline) {
    await page.waitForTimeout(500);
  }

  if (!created) {
    created = await page.evaluate(async () => {
      const res = await fetch("https://spamchecker-api.mailreach.co/api/v1/tests?", {
        method: "POST",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        body: "{}",
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(`HTTP ${res.status}: ${text.slice(0, 200)}`);
      }
      return await res.json();
    });
  }

  if (!created?.public_id) fail("create returned no public_id");
  process.stdout.write(JSON.stringify(created));
} catch (err) {
  fail(String(err?.message || err));
} finally {
  await browser.close().catch(() => {});
}

async function readStdin() {
  const chunks = [];
  for await (const c of process.stdin) chunks.push(c);
  return Buffer.concat(chunks).toString("utf8") || "{}";
}

function fail(msg) {
  console.error(msg);
  process.exit(1);
}
