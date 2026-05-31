#!/usr/bin/env node
/**
 * Records an English walkthrough of the Cognitive Graph UI (on-screen captions).
 *
 *   docker compose --profile graph -f docker-compose.yml -f docker-compose.record-graph.yml up -d api
 *   node scripts/e2e/record_graph_ui_demo.mjs
 *   → docs/assets/graph-ui-demo.mp4
 */
import { chromium } from 'playwright';
import { mkdir, rm } from 'fs/promises';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import { execFile } from 'child_process';
import { promisify } from 'util';

const execFileAsync = promisify(execFile);
const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '../..');
const OUT_DIR = join(ROOT, 'docs/assets');
const OUT_FILE = join(OUT_DIR, 'graph-ui-demo.mp4');
const BASE = process.env.PCMI_BASE_URL || 'http://localhost:8000';
const API_KEY = process.env.PCMI_API_KEY || 'testkey123';
const UI = `${BASE}/v1/graph/ui?record=1`;
const CAPTURE_FPS = 3.5;

const pause = (ms) => new Promise((r) => setTimeout(r, ms));
let frameIdx = 0;

function esc(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

async function shot(page, framesDir) {
  const name = `frame_${String(++frameIdx).padStart(5, '0')}.png`;
  await page.screenshot({ path: join(framesDir, name), fullPage: false });
}

async function caption(page, step, title, body) {
  await page.evaluate(({ step, title, body }) => {
    window.pcmiRecord?.caption(step, title, body);
  }, { step, title, body });
}

async function recordScene(page, framesDir, { dwellMs, step, title, body, run }) {
  await caption(page, step, title, body);
  if (run) await run(page);
  const frames = Math.max(3, Math.round((dwellMs / 1000) * CAPTURE_FPS));
  const interval = dwellMs / frames;
  for (let i = 0; i < frames; i++) {
    await pause(interval);
    await shot(page, framesDir);
  }
}

async function titleSlide(page, framesDir, { step, title, body, dwellMs = 5200 }) {
  const html = `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8">
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{height:100vh;display:grid;place-items:center;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;
    background:radial-gradient(1000px 520px at 75% -5%,rgba(45,212,191,.14),transparent 58%),
    radial-gradient(800px 480px at -5% 105%,rgba(56,189,248,.1),transparent 52%),#0a0f1e;color:#e6edf7}
  .card{max-width:920px;padding:52px 60px;text-align:center}
  .step{font-size:11px;font-weight:700;letter-spacing:.16em;text-transform:uppercase;color:#2dd4bf;margin-bottom:16px}
  h1{font-size:36px;font-weight:650;line-height:1.18;margin-bottom:18px;letter-spacing:-.02em}
  p{font-size:18px;color:#94a3b8;line-height:1.65;max-width:780px;margin:0 auto}
  .logo{width:52px;height:52px;border-radius:14px;margin:0 auto 24px;
    background:linear-gradient(135deg,#2dd4bf,#38bdf8);box-shadow:0 10px 32px rgba(45,212,191,.4)}
  .pill{display:inline-block;margin-top:22px;font-size:12px;font-weight:600;letter-spacing:.06em;
    padding:6px 14px;border-radius:999px;border:1px solid rgba(45,212,191,.35);color:#5eead4;background:rgba(45,212,191,.08)}
</style></head><body>
<div class="card"><div class="logo"></div>
  <div class="step">${esc(step)}</div>
  <h1>${esc(title)}</h1>
  <p>${esc(body)}</p>
  <div class="pill">Apache AGE · REST /v1/graph/*</div>
</div></body></html>`;
  await page.setContent(html, { waitUntil: 'domcontentloaded' });
  const frames = Math.round((dwellMs / 1000) * CAPTURE_FPS);
  for (let i = 0; i < frames; i++) {
    await pause(dwellMs / frames);
    await shot(page, framesDir);
  }
}

async function setExplore(page, { memId, depth, linkTypes }) {
  await page.evaluate(({ memId, depth, linkTypes }) => {
    window.pcmiRecord.setMemory(memId, depth);
    window.pcmiRecord.setLinkTypes(linkTypes);
  }, { memId, depth, linkTypes });
}

async function resolveChainTarget(from, candidates) {
  for (const to of candidates) {
    const url = `${BASE}/v1/graph/chain?from=${from}&to=${to}&link_types=causal&max_depth=12`;
    const data = await fetch(url, { headers: { 'X-API-Key': API_KEY } }).then((r) => r.json());
    if (data.connected) return { to, hops: data.hops };
  }
  return { to: candidates[candidates.length - 1], hops: 0 };
}

async function main() {
  await mkdir(OUT_DIR, { recursive: true });
  const framesDir = join(OUT_DIR, '.record-frames');
  await rm(framesDir, { recursive: true, force: true });
  await mkdir(framesDir, { recursive: true });

  const health = await fetch(`${BASE}/v1/graph/health`).then((r) => r.json()).catch(() => ({}));
  if (!health.available) {
    console.error('Apache AGE is not available. Start API against postgres-age:');
    console.error('  docker compose --profile graph -f docker-compose.yml -f docker-compose.record-graph.yml up -d api');
    process.exit(1);
  }

  const chain = await resolveChainTarget(14, [22, 18]);
  console.log('Recording →', OUT_FILE, `(chain 14→${chain.to}, hops=${chain.hops})`);

  const launchOpts = { headless: true };
  if (process.env.PW_CHANNEL) launchOpts.channel = process.env.PW_CHANNEL;
  else if (process.platform === 'darwin') launchOpts.channel = 'chrome';

  const browser = await chromium.launch(launchOpts);
  const context = await browser.newContext({
    viewport: { width: 1280, height: 720 },
    colorScheme: 'dark',
  });
  const page = await context.newPage();

  // ── Opening title ─────────────────────────────────────────────────────
  await titleSlide(page, framesDir, {
    step: 'PCMI Cognitive Graph',
    title: 'Turn SOC memories into an explorable graph',
    body: 'Each node is a PCMI memory (alert or incident). Typed edges — causal, temporal, contradicts, supports, related — power multi-hop traversal and kill-chain reconstruction on Apache AGE.',
    dwellMs: 5800,
  });

  await page.goto(UI, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('#btn-explore', { timeout: 30000 });
  await page.waitForFunction(() => window.pcmiRecord != null, { timeout: 10000 });

  await recordScene(page, framesDir, {
    dwellMs: 5200,
    step: '01 · Explorer',
    title: 'Live graph over your memory store',
    body: 'The UI calls the same REST endpoints you use in production: GET /v1/graph/related for neighborhood expansion and GET /v1/graph/chain for shortest-path reconstruction between two memories.',
    run: async (p) => {
      await p.evaluate(() => window.pcmiRecord.highlight('.canvas-wrap', true));
      await pause(1200);
      await p.evaluate(() => window.pcmiRecord.highlight('.canvas-wrap', false));
    },
  });

  await recordScene(page, framesDir, {
    dwellMs: 4800,
    step: '02 · Connect',
    title: 'API key + AGE health',
    body: 'Authenticate with your key. A green badge means postgres-age is wired in — graph traversal and /chain run on the property graph; without AGE those endpoints are degraded or unavailable.',
    run: async (p) => {
      await p.evaluate(() => {
        window.pcmiRecord.highlight('.api-key-area', true);
        window.pcmiRecord.highlight('#age-status', true);
      });
      await p.locator('#api-key').click();
      await p.locator('#api-key').fill('');
      await p.locator('#api-key').type(API_KEY, { delay: 40 });
      await pause(900);
      await p.evaluate(() => {
        window.pcmiRecord.highlight('.api-key-area', false);
        window.pcmiRecord.highlight('#age-status', false);
      });
    },
  });

  // ── CAMP000004 kill chain (memory 14) ───────────────────────────────────
  await recordScene(page, framesDir, {
    dwellMs: 7200,
    step: '03 · Traversal',
    title: 'Kill chain: CAMP000004 (memory 14, depth 5)',
    body: 'Filter causal + temporal edges. Each hop is labeled with link type and painted by depth (see legend). This is the Conti campaign: resource development through post-incident stages — severity ramps P3→P1 across the chain.',
    run: async (p) => {
      await setExplore(p, { memId: 14, depth: 5, linkTypes: 'causal,temporal' });
      await p.evaluate(() => {
        window.pcmiRecord.highlight('#mem-id', true);
        window.pcmiRecord.highlight('#depth', true);
        window.pcmiRecord.highlight('#link-types', true);
        window.pcmiRecord.highlight('.legend', true);
      });
      await pause(1100);
      await p.click('#btn-explore');
      await p.evaluate(() => window.pcmiRecord.waitNodes(8));
      await p.evaluate(() => {
        ['#mem-id', '#depth', '#link-types', '.legend'].forEach((s) => window.pcmiRecord.highlight(s, false));
      });
      await pause(1800);
    },
  });

  await recordScene(page, framesDir, {
    dwellMs: 4500,
    step: '04 · Layouts',
    title: 'Tree and radial views',
    body: 'Force layout shows natural clusters. Tree orders nodes by hop depth for stage-by-stage briefings. Radial places the root at the center — ideal when presenting a single campaign to leadership.',
    run: async (p) => {
      await p.click('#btn-tree');
      await pause(2000);
      await p.click('#btn-radial');
      await pause(2400);
      await p.click('#btn-force');
      await pause(800);
    },
  });

  await recordScene(page, framesDir, {
    dwellMs: 6200,
    step: '05 · Inspector',
    title: 'Memory detail drawer',
    body: 'Click any node to open the inspector: hierarchical path, alert content, tags, and metadata from the graph memory cache. Severity, MITRE, and link context appear as badges — no context switching to another tool.',
    run: async (p) => {
      const nodes = await p.evaluate(() => parseInt(document.getElementById('stat-nodes')?.textContent || '0', 10));
      if (nodes < 4) {
        await setExplore(p, { memId: 14, depth: 5, linkTypes: 'causal,temporal' });
        await p.click('#btn-explore');
        await p.evaluate(() => window.pcmiRecord.waitNodes(6));
      }
      await p.evaluate(() => window.pcmiRecord.pickNode(17));
      await p.waitForSelector('#inspector[aria-hidden="false"]', { timeout: 12000 });
      await p.evaluate(() => window.pcmiRecord.highlight('#inspector', true));
      await pause(2400);
      await p.evaluate(() => window.pcmiRecord.highlight('#inspector', false));
    },
  });

  await recordScene(page, framesDir, {
    dwellMs: 7500,
    step: '06 · Shortest path',
    title: `Causal chain: memory 14 → ${chain.to}`,
    body: `Find Chain calls GET /v1/graph/chain. The shortest causal path is highlighted in gold (${chain.hops || 'multi'}-hop). Analysts use this to prove stage connectivity or explain why two alerts belong to the same incident.`,
    run: async (p) => {
      await p.click('#btn-clear');
      await pause(400);
      await setExplore(p, { memId: 14, depth: 5, linkTypes: 'causal' });
      await p.click('#btn-explore');
      await p.evaluate(() => window.pcmiRecord.waitNodes(6));
      await p.evaluate((to) => window.pcmiRecord.chainTo(to), chain.to);
      await pause(3200);
    },
  });

  await recordScene(page, framesDir, {
    dwellMs: 6800,
    step: '07 · Edge types',
    title: 'Five semantic link types on one canvas',
    body: 'Red = causal (kill chain). Amber = temporal ordering. Pink = contradicts (e.g. false positive overturned by confirmed activity). Green = supports (postmortem back-links). Purple = related (shared actor, duplicate storms, cross-campaign ties).',
    run: async (p) => {
      await p.evaluate(() =>
        window.pcmiRecord.clearAndExplore(14, 5, 'causal,temporal,contradicts,supports,related'),
      );
      await p.evaluate(() => window.pcmiRecord.waitNodes(10));
      await p.evaluate(() => window.pcmiRecord.highlight('#link-types', true));
      await pause(2200);
      await p.evaluate(() => window.pcmiRecord.highlight('#link-types', false));
    },
  });

  // ── Second campaign + pivot ─────────────────────────────────────────────
  await recordScene(page, framesDir, {
    dwellMs: 6800,
    step: '08 · Second campaign',
    title: 'CAMP000007 Royal (memory 35)',
    body: 'The same workflow scales to another intrusion set: persistence → privilege escalation → lateral movement → exfiltration. Shared host/user/IP metadata stays coherent across hops — the graph preserves analyst narrative, not just isolated alerts.',
    run: async (p) => {
      await p.click('#btn-clear');
      await pause(400);
      await setExplore(p, { memId: 35, depth: 5, linkTypes: 'causal,temporal' });
      await p.click('#btn-explore');
      await p.evaluate(() => window.pcmiRecord.waitNodes(12));
      await p.click('#btn-radial');
      await pause(2200);
    },
  });

  await recordScene(page, framesDir, {
    dwellMs: 5800,
    step: '09 · Expand',
    title: 'Explore from any node',
    body: 'Double-click a node — or use Explore from here in the inspector — to re-root the traversal. Pivot across campaigns without re-entering IDs manually; the graph grows incrementally on the canvas.',
    run: async (p) => {
      await p.evaluate(() => window.pcmiRecord.pickNode(35));
      await pause(600);
      await p.evaluate(() => window.pcmiRecord.exploreFrom(40));
      await p.evaluate(() => window.pcmiRecord.waitNodes(15));
      await pause(2000);
    },
  });

  await recordScene(page, framesDir, {
    dwellMs: 6200,
    step: '10 · Postmortem',
    title: 'Supports fan-out (memory 22, depth 1)',
    body: 'Postmortem / synthesis nodes link backward with supports edges to every stage they summarize. Depth 1 from the campaign tail reveals the full fan — how leadership reporting ties back to tactical alerts.',
    run: async (p) => {
      await p.click('#btn-clear');
      await pause(400);
      await p.evaluate(() => window.pcmiRecord.clearAndExplore(22, 1, 'supports'));
      await p.evaluate(() => window.pcmiRecord.waitNodes(8));
      await p.click('#btn-force');
      await pause(2200);
    },
  });

  await recordScene(page, framesDir, {
    dwellMs: 5800,
    step: '11 · Cross-campaign',
    title: 'Related edges across campaigns',
    body: 'Selecting related-only surfaces ties between incidents that share threat actors or infrastructure — purple edges crossing campaign boundaries. This is how you connect parallel intrusions without merging them into one false narrative.',
    run: async (p) => {
      await p.click('#btn-clear');
      await pause(400);
      await p.evaluate(() => window.pcmiRecord.clearAndExplore(100, 4, 'related'));
      await p.evaluate(() => window.pcmiRecord.waitNodes(10));
      await pause(2000);
    },
  });

  await recordScene(page, framesDir, {
    dwellMs: 5200,
    step: '12 · Clusters',
    title: 'Cluster view for alert storms',
    body: 'Toggle Clusters to collapse nodes that share a path prefix — dozens of duplicate alerts on the same subnet become one expandable group instead of unreadable hairballs in force layout.',
    run: async (p) => {
      await p.evaluate(() => window.pcmiRecord.clearAndExplore(35, 5, 'causal,temporal,related,contradicts'));
      await p.evaluate(() => window.pcmiRecord.waitNodes(12));
      const on = await p.locator('#btn-clusters').evaluate((el) => el.classList.contains('btn-active'));
      if (!on) await p.click('#btn-clusters');
      await pause(2800);
    },
  });

  await recordScene(page, framesDir, {
    dwellMs: 5200,
    step: '13 · Timeline',
    title: 'Temporal axis',
    body: 'The timeline maps each loaded memory by timestamp. Click a dot to focus that node — correlate dwell time, business hours, and stage order with the graph structure above.',
    run: async (p) => {
      const on = await p.locator('#btn-timeline').evaluate((el) => el.classList.contains('btn-active'));
      if (!on) await p.click('#btn-timeline');
      await pause(2800);
    },
  });

  // ── Strong close (no setup outro) ───────────────────────────────────────
  await recordScene(page, framesDir, {
    dwellMs: 6000,
    step: '14 · Summary',
    title: 'Graph-native SOC analysis in PCMI',
    body: 'Traversal, shortest-path chains, typed semantics, inspector, layouts, clusters, and timeline — one surface backed by Apache AGE and your existing memory API. Built for analysts who need the story, not just the search hit.',
    run: async (p) => {
      await p.click('#btn-clear');
      await pause(300);
      await setExplore(p, { memId: 14, depth: 5, linkTypes: 'causal,temporal,supports,related' });
      await p.click('#btn-explore');
      await p.evaluate(() => window.pcmiRecord.waitNodes(15));
      await p.click('#btn-force');
      await p.evaluate(() => window.pcmiRecord.highlight('.header', true));
      await pause(2000);
      await p.evaluate(() => window.pcmiRecord.highlight('.header', false));
    },
  });

  await context.close();
  await browser.close();

  const fps = CAPTURE_FPS.toFixed(2);
  await execFileAsync('ffmpeg', [
    '-y', '-framerate', fps, '-i', 'frame_%05d.png',
    '-vf', 'fps=30,format=yuv420p',
    '-c:v', 'libx264', '-preset', 'medium', '-crf', '19',
    '-movflags', '+faststart',
    OUT_FILE,
  ], { cwd: framesDir });
  await rm(framesDir, { recursive: true, force: true });

  const { stdout } = await execFileAsync('ffprobe', [
    '-v', 'error', '-show_entries', 'format=duration', '-of', 'default=noprint_wrappers=1:nokey=1',
    OUT_FILE,
  ]);
  console.log('Done:', OUT_FILE, `— ${parseFloat(stdout).toFixed(0)}s, ${frameIdx} frames`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
