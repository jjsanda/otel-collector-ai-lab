// Capture screenshots of the live stack (Grafana dashboard + a Jaeger trace)
// into docs/screenshots/. Requires the Compose stack to be running and Playwright
// (`npm i playwright && npx playwright install chromium`). Run: node scripts/screenshots.mjs
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";

const OUT = "docs/screenshots";
const GRAFANA = process.env.GRAFANA_URL || "http://localhost:3000";
const JAEGER = process.env.JAEGER_URL || "http://localhost:16686";

mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch({ args: ["--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage"] });
const page = await browser.newPage({ viewport: { width: 1600, height: 1200 } });

// --- Grafana: GenAI overview dashboard (kiosk mode, anonymous admin) ---
const dash = `${GRAFANA}/d/genai-overview/genai-telemetry-overview?kiosk&from=now-30m&to=now`;
await page.goto(dash, { waitUntil: "networkidle", timeout: 60000 });
// Grafana renders panels asynchronously — wait for the grid, then for values.
await page.waitForSelector(".react-grid-item", { timeout: 30000 }).catch(() => {});
await page.waitForFunction(() => document.querySelectorAll(".react-grid-item").length >= 4, { timeout: 30000 }).catch(() => {});
await page.waitForTimeout(9000);
// Grafana panels live in a nested scroll container, so fullPage captures only
// the toolbar; capture the tall viewport instead.
await page.screenshot({ path: `${OUT}/grafana-genai-overview.png`, fullPage: false });
console.log("captured grafana-genai-overview.png");

// --- Jaeger: the richest trace under service "api" ---
try {
  const res = await fetch(`${JAEGER}/api/traces?service=api&limit=20`);
  const body = await res.json();
  let best = null;
  let bestN = 0;
  for (const t of body.data || []) {
    if (t.spans.length > bestN) {
      bestN = t.spans.length;
      best = t.traceID;
    }
  }
  if (best) {
    await page.goto(`${JAEGER}/trace/${best}`, { waitUntil: "networkidle", timeout: 60000 });
    await page.waitForTimeout(4000);
    await page.screenshot({ path: `${OUT}/jaeger-trace.png`, fullPage: true });
    console.log(`captured jaeger-trace.png (trace ${best}, ${bestN} spans)`);
  } else {
    console.error("no trace found for service=api — generate load first");
  }
} catch (err) {
  console.error("jaeger capture failed:", err.message);
}

await browser.close();
