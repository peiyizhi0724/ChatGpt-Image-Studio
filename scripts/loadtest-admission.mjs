#!/usr/bin/env node

import { existsSync } from 'node:fs';
import fs from 'node:fs/promises';
import http from 'node:http';
import https from 'node:https';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, '..');

async function main() {
  const { options } = parseArgs(process.argv.slice(2));

  const hints = await loadLocalConfigHints(repoRoot);
  const baseUrl = normalizeBaseUrl(
    firstNonEmpty(options['base-url'], process.env.STUDIO_BASE_URL, defaultBaseUrl(hints)),
  );
  const authKey = firstNonEmpty(options['auth-key'], process.env.STUDIO_AUTH_KEY, hints.authKey);
  const timeoutMS = parseIntWithDefault(options['timeout-ms'], 120000);

  if (!authKey) {
    throw new Error(
      'Missing auth key. Use --auth-key, STUDIO_AUTH_KEY, or set app.auth_key in backend/data/config.toml.',
    );
  }

  const payload = {
    total: clamp(parseIntWithDefault(options.total, 120), 1, 5000),
    workers: clamp(parseIntWithDefault(options.workers, 20), 1, 200),
    holdMs: clamp(parseIntWithDefault(options['hold-ms'], 1200), 0, 120000),
    timeoutSeconds: clamp(parseIntWithDefault(options['timeout-seconds'], 60), 5, 900),
  };

  const response = await requestJSON({
    method: 'POST',
    url: `${baseUrl}/api/tools/admission-stress`,
    authKey,
    timeoutMS,
    body: payload,
  });

  if (response.statusCode >= 400) {
    if (response.statusCode === 404) {
      throw new Error(
        'Endpoint /api/tools/admission-stress is not available. Restart backend with the latest code first.',
      );
    }
    throw new Error(
      `POST /api/tools/admission-stress failed (${response.statusCode}): ${extractAPIError(response.body)}`,
    );
  }

  const data = response.body || {};
  const counters = data.counters || {};
  const queueWait = data.queueWait || {};
  const attempted = Number(counters.attempted || 0);
  const admitted = Number(counters.admitted || 0);
  const admissionRate = attempted > 0 ? admitted / attempted : 0;

  console.log('Admission load test result');
  console.log(`  startedAt: ${data.startedAt || '-'}`);
  console.log(`  finishedAt: ${data.finishedAt || '-'}`);
  console.log(`  durationMs: ${data.durationMs ?? '-'}`);
  console.log(`  timedOut: ${Boolean(data.timedOut)}`);
  console.log('');
  console.log('Counters');
  console.log(`  submitted: ${counters.submitted ?? 0}`);
  console.log(`  attempted: ${counters.attempted ?? 0}`);
  console.log(`  admitted: ${counters.admitted ?? 0}`);
  console.log(`  queueFull: ${counters.queueFull ?? 0}`);
  console.log(`  queueTimeout: ${counters.queueTimeout ?? 0}`);
  console.log(`  canceled: ${counters.canceled ?? 0}`);
  console.log(`  otherErrors: ${counters.otherErrors ?? 0}`);
  console.log('');
  console.log('Queue wait');
  console.log(`  samples: ${queueWait.samples ?? 0}`);
  console.log(`  avgMs: ${formatFloat(queueWait.avgMs)}`);
  console.log(`  p50Ms: ${queueWait.p50Ms ?? 0}`);
  console.log(`  p95Ms: ${queueWait.p95Ms ?? 0}`);
  console.log(`  maxMs: ${queueWait.maxMs ?? 0}`);
  console.log('');

  if (admissionRate >= 0.9 && Number(counters.queueTimeout || 0) === 0 && !data.timedOut) {
    console.log('Verdict: PASS (admission throughput is healthy for this profile).');
  } else if (admissionRate >= 0.6) {
    console.log('Verdict: WARN (throughput is acceptable but saturation is visible).');
  } else {
    console.log('Verdict: FAIL (admission is heavily saturated, lower load or adjust profile).');
  }
}

function parseArgs(argv) {
  const positional = [];
  const options = {};
  for (let i = 0; i < argv.length; i += 1) {
    const token = argv[i];
    if (!token.startsWith('--')) {
      positional.push(token);
      continue;
    }
    const eq = token.indexOf('=');
    if (eq > -1) {
      options[token.slice(2, eq)] = token.slice(eq + 1);
      continue;
    }
    const key = token.slice(2);
    const next = argv[i + 1];
    if (next && !next.startsWith('--')) {
      options[key] = next;
      i += 1;
      continue;
    }
    options[key] = 'true';
  }
  return { positional, options };
}

async function loadLocalConfigHints(rootDir) {
  const candidates = [
    path.join(rootDir, 'backend', 'data', 'config.toml'),
    path.join(rootDir, 'backend', 'data', 'config.example.toml'),
  ];
  let merged = {};
  for (const file of candidates) {
    if (!existsSync(file)) {
      continue;
    }
    const raw = await fs.readFile(file, 'utf8');
    merged = mergeTomlSections(merged, parseTomlSections(raw));
  }
  return {
    host: normalizeHost(merged?.server?.host),
    port: parseIntWithDefault(merged?.server?.port, 7000),
    authKey: stringOrEmpty(merged?.app?.auth_key),
  };
}

function parseTomlSections(raw) {
  const out = {};
  let section = '';
  for (const lineRaw of raw.split(/\r?\n/u)) {
    const line = lineRaw.trim();
    if (!line || line.startsWith('#')) {
      continue;
    }
    const sectionMatch = line.match(/^\[(.+)\]$/u);
    if (sectionMatch) {
      section = sectionMatch[1].trim();
      if (!out[section]) {
        out[section] = {};
      }
      continue;
    }
    const kvMatch = line.match(/^([A-Za-z0-9_]+)\s*=\s*(.+)$/u);
    if (!kvMatch || !section) {
      continue;
    }
    const key = kvMatch[1].trim();
    const parsedValue = parseTomlValue(kvMatch[2].trim());
    out[section][key] = parsedValue;
  }
  return out;
}

function parseTomlValue(raw) {
  const trimmed = raw.trim();
  const withoutComment = trimmed.replace(/\s+#.*$/u, '').trim();
  if (
    (withoutComment.startsWith('"') && withoutComment.endsWith('"')) ||
    (withoutComment.startsWith("'") && withoutComment.endsWith("'"))
  ) {
    return withoutComment.slice(1, -1);
  }
  if (/^(true|false)$/iu.test(withoutComment)) {
    return /^true$/iu.test(withoutComment);
  }
  if (/^-?\d+$/u.test(withoutComment)) {
    return Number(withoutComment);
  }
  return withoutComment;
}

function mergeTomlSections(base, next) {
  const merged = { ...base };
  for (const [section, values] of Object.entries(next)) {
    merged[section] = {
      ...(merged[section] || {}),
      ...values,
    };
  }
  return merged;
}

function defaultBaseUrl(hints) {
  return `http://${normalizeHost(hints.host)}:${parseIntWithDefault(hints.port, 7000)}`;
}

function normalizeHost(host) {
  const value = stringOrEmpty(host);
  if (!value || value === '0.0.0.0' || value === '::') {
    return '127.0.0.1';
  }
  return value;
}

function normalizeBaseUrl(baseUrl) {
  const value = stringOrEmpty(baseUrl);
  if (!value) {
    return 'http://127.0.0.1:7000';
  }
  return value.replace(/\/+$/u, '');
}

function requestJSON({ method, url, authKey, timeoutMS, body }) {
  const target = new URL(url);
  const transport = target.protocol === 'https:' ? https : http;
  const payload = body == null ? '' : JSON.stringify(body);

  return new Promise((resolve, reject) => {
    const req = transport.request(
      {
        protocol: target.protocol,
        hostname: target.hostname,
        port: target.port || undefined,
        path: `${target.pathname}${target.search}`,
        method,
        headers: {
          Authorization: `Bearer ${authKey}`,
          Accept: 'application/json',
          ...(payload
            ? {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(payload),
              }
            : {}),
        },
      },
      (res) => {
        const chunks = [];
        res.on('data', (chunk) => chunks.push(chunk));
        res.on('end', () => {
          const raw = Buffer.concat(chunks).toString('utf8');
          let parsed = null;
          try {
            parsed = raw ? JSON.parse(raw) : null;
          } catch {
            parsed = { raw };
          }
          resolve({
            statusCode: res.statusCode || 0,
            body: parsed,
          });
        });
      },
    );

    req.setTimeout(timeoutMS, () => {
      req.destroy(new Error(`request timeout after ${timeoutMS}ms`));
    });
    req.on('error', reject);
    if (payload) {
      req.write(payload);
    }
    req.end();
  });
}

function extractAPIError(body) {
  if (!body) {
    return 'empty response';
  }
  if (typeof body.error === 'string') {
    return body.error;
  }
  if (typeof body.message === 'string') {
    return body.message;
  }
  if (typeof body?.error?.message === 'string') {
    return body.error.message;
  }
  return JSON.stringify(body);
}

function parseIntWithDefault(value, fallback) {
  const parsed = Number.parseInt(String(value ?? ''), 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function formatFloat(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) {
    return '0.00';
  }
  return numeric.toFixed(2);
}

function stringOrEmpty(value) {
  return typeof value === 'string' ? value.trim() : '';
}

function firstNonEmpty(...values) {
  for (const value of values) {
    const normalized = stringOrEmpty(value);
    if (normalized) {
      return normalized;
    }
  }
  return '';
}

main().catch((err) => {
  console.error(err?.message || String(err));
  process.exitCode = 1;
});
