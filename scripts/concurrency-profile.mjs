#!/usr/bin/env node

import { existsSync } from 'node:fs';
import fs from 'node:fs/promises';
import http from 'node:http';
import https from 'node:https';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const PRESETS = {
  high20: {
    server: {
      maxImageConcurrency: 20,
      imageQueueLimit: 80,
      imageQueueTimeoutSeconds: 25,
    },
    accounts: {
      imageQuotaRefreshTTLSeconds: 120,
    },
  },
  safe8: {
    server: {
      maxImageConcurrency: 8,
      imageQueueLimit: 32,
      imageQueueTimeoutSeconds: 20,
    },
    accounts: {
      imageQuotaRefreshTTLSeconds: 120,
    },
  },
};

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, '..');

async function main() {
  const { positional, options } = parseArgs(process.argv.slice(2));
  const action = (positional[0] || 'show').toLowerCase();

  if (!['show', 'high20', 'safe8', 'rollback'].includes(action)) {
    printUsage();
    process.exitCode = 1;
    return;
  }

  const configHints = await loadLocalConfigHints(repoRoot);
  const baseUrl = normalizeBaseUrl(
    firstNonEmpty(
      options['base-url'],
      process.env.STUDIO_BASE_URL,
      defaultBaseUrl(configHints),
    ),
  );
  const authKey = firstNonEmpty(
    options['auth-key'],
    process.env.STUDIO_AUTH_KEY,
    configHints.authKey,
  );
  const timeoutMS = parseIntWithDefault(options['timeout-ms'], 10000);
  const backupFile = path.resolve(
    options['backup-file'] || path.join(repoRoot, '.runtime', 'concurrency-profile-backup.json'),
  );

  if (!authKey) {
    throw new Error(
      'Missing auth key. Use --auth-key, STUDIO_AUTH_KEY, or set app.auth_key in backend/data/config.toml.',
    );
  }

  if (action === 'show') {
    const currentConfig = await getConfig(baseUrl, authKey, timeoutMS);
    const profile = extractProfile(currentConfig);
    printProfile('Current profile', profile);
    console.log('');
    console.log('Presets:');
    printProfile('high20', PRESETS.high20);
    printProfile('safe8', PRESETS.safe8);
    console.log('');
    console.log('Examples:');
    console.log('  node scripts/concurrency-profile.mjs high20');
    console.log('  node scripts/concurrency-profile.mjs rollback');
    return;
  }

  if (action === 'rollback') {
    if (!existsSync(backupFile)) {
      throw new Error(`Backup not found: ${backupFile}`);
    }
    const backupRaw = await fs.readFile(backupFile, 'utf8');
    const backup = JSON.parse(backupRaw);
    const rollbackProfile = backup?.profile;
    if (!rollbackProfile?.server || !rollbackProfile?.accounts) {
      throw new Error(`Backup file is invalid: ${backupFile}`);
    }

    const currentConfig = await getConfig(baseUrl, authKey, timeoutMS);
    const nextConfig = deepClone(currentConfig);
    applyProfile(nextConfig, rollbackProfile);
    await putConfig(baseUrl, authKey, timeoutMS, nextConfig);

    console.log(`Rolled back from backup: ${backupFile}`);
    printProfile('Rolled back profile', rollbackProfile);
    return;
  }

  const preset = PRESETS[action];
  const currentConfig = await getConfig(baseUrl, authKey, timeoutMS);
  const currentProfile = extractProfile(currentConfig);
  await writeBackup(backupFile, {
    savedAt: new Date().toISOString(),
    sourceAction: action,
    baseUrl,
    profile: currentProfile,
  });

  const nextConfig = deepClone(currentConfig);
  applyProfile(nextConfig, preset);
  await putConfig(baseUrl, authKey, timeoutMS, nextConfig);

  console.log(`Applied preset: ${action}`);
  printProfile('Applied profile', preset);
  console.log(`Backup saved: ${backupFile}`);
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

function extractProfile(configPayload) {
  return {
    server: {
      maxImageConcurrency: parseIntWithDefault(configPayload?.server?.maxImageConcurrency, 8),
      imageQueueLimit: parseIntWithDefault(configPayload?.server?.imageQueueLimit, 32),
      imageQueueTimeoutSeconds: parseIntWithDefault(
        configPayload?.server?.imageQueueTimeoutSeconds,
        20,
      ),
    },
    accounts: {
      imageQuotaRefreshTTLSeconds: parseIntWithDefault(
        configPayload?.accounts?.imageQuotaRefreshTTLSeconds,
        120,
      ),
    },
  };
}

function applyProfile(configPayload, profile) {
  if (!configPayload.server) {
    configPayload.server = {};
  }
  if (!configPayload.accounts) {
    configPayload.accounts = {};
  }
  configPayload.server.maxImageConcurrency = profile.server.maxImageConcurrency;
  configPayload.server.imageQueueLimit = profile.server.imageQueueLimit;
  configPayload.server.imageQueueTimeoutSeconds = profile.server.imageQueueTimeoutSeconds;
  configPayload.accounts.imageQuotaRefreshTTLSeconds = profile.accounts.imageQuotaRefreshTTLSeconds;
}

function printProfile(label, profile) {
  console.log(`${label}:`);
  console.log(`  maxImageConcurrency: ${profile.server.maxImageConcurrency}`);
  console.log(`  imageQueueLimit: ${profile.server.imageQueueLimit}`);
  console.log(`  imageQueueTimeoutSeconds: ${profile.server.imageQueueTimeoutSeconds}`);
  console.log(`  imageQuotaRefreshTTLSeconds: ${profile.accounts.imageQuotaRefreshTTLSeconds}`);
}

async function writeBackup(backupFile, payload) {
  await fs.mkdir(path.dirname(backupFile), { recursive: true });
  await fs.writeFile(backupFile, `${JSON.stringify(payload, null, 2)}\n`, 'utf8');
}

async function getConfig(baseUrl, authKey, timeoutMS) {
  const response = await requestJSON({
    method: 'GET',
    url: `${baseUrl}/api/config`,
    authKey,
    timeoutMS,
  });
  if (response.statusCode >= 400) {
    throw new Error(`GET /api/config failed (${response.statusCode}): ${extractAPIError(response.body)}`);
  }
  return response.body;
}

async function putConfig(baseUrl, authKey, timeoutMS, payload) {
  const response = await requestJSON({
    method: 'PUT',
    url: `${baseUrl}/api/config`,
    authKey,
    timeoutMS,
    body: payload,
  });
  if (response.statusCode >= 400) {
    throw new Error(`PUT /api/config failed (${response.statusCode}): ${extractAPIError(response.body)}`);
  }
  return response.body;
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

function deepClone(value) {
  return JSON.parse(JSON.stringify(value));
}

function printUsage() {
  console.log('Usage:');
  console.log('  node scripts/concurrency-profile.mjs show');
  console.log('  node scripts/concurrency-profile.mjs high20');
  console.log('  node scripts/concurrency-profile.mjs safe8');
  console.log('  node scripts/concurrency-profile.mjs rollback');
  console.log('');
  console.log('Options:');
  console.log('  --base-url http://127.0.0.1:7000');
  console.log('  --auth-key <app.auth_key>');
  console.log('  --backup-file .runtime/concurrency-profile-backup.json');
  console.log('  --timeout-ms 10000');
}

main().catch((err) => {
  console.error(err?.message || String(err));
  process.exitCode = 1;
});
