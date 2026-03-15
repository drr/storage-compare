'use strict';

const { v4: uuidv4 } = require('uuid');

function generateContent(rng) {
  const words = ['the', 'be', 'to', 'of', 'and', 'a', 'in', 'that', 'have', 'it',
    'for', 'not', 'on', 'with', 'he', 'as', 'you', 'do', 'at', 'this',
    'but', 'his', 'by', 'from', 'they', 'we', 'say', 'her', 'she', 'or',
    'an', 'will', 'my', 'one', 'all', 'would', 'there', 'their', 'what', 'so',
    'up', 'out', 'if', 'about', 'who', 'get', 'which', 'go', 'me', 'when',
    'make', 'can', 'like', 'time', 'no', 'just', 'him', 'know', 'take',
    'people', 'into', 'year', 'your', 'good', 'some', 'could', 'them', 'see',
    'other', 'than', 'then', 'now', 'look', 'only', 'come', 'its', 'over'];
  const n = Math.max(10, Math.min(500, Math.round(Math.exp(3.5 + rng() * 1.5))));
  let out = '';
  for (let i = 0; i < n; i++) {
    if (i > 0 && i % 60 === 0) out += '\n\n';
    else if (i > 0) out += ' ';
    out += words[Math.floor(rng() * words.length)];
  }
  return out + '.\n';
}

// Returns an Op: () => BigInt latency in ns
function readRandomOp(db, index, rng) {
  const stmt = db.prepare(
    'SELECT id, version_id, entry_type, create_time, modify_time, is_latest, content FROM entries WHERE id = ? AND is_latest = 1'
  );
  return () => {
    const entry = index[Math.floor(rng() * index.length)];
    const t0 = process.hrtime.bigint();
    stmt.get(entry.id);
    return process.hrtime.bigint() - t0;
  };
}

// Returns an Op: () => BigInt latency in ns
function readDayOp(db, dayPool, rng) {
  const days = [...dayPool.keys()];
  const stmt = db.prepare(
    'SELECT id, version_id, entry_type, create_time, modify_time, is_latest, content FROM entries WHERE modify_time >= ? AND modify_time < ? AND is_latest = 1'
  );
  return () => {
    const day = days[Math.floor(rng() * days.length)];
    const t = new Date(day + 'T00:00:00Z').getTime();
    const t0 = process.hrtime.bigint();
    stmt.all(t, t + 86400000);
    return process.hrtime.bigint() - t0;
  };
}

// Returns an Op: () => BigInt latency in ns (unlimited, no pool)
function createEntryOp(db, rng) {
  const stmt = db.prepare(
    'INSERT INTO entries (id, version_id, entry_type, create_time, modify_time, is_latest, content) VALUES (?, ?, ?, ?, ?, 1, ?)'
  );
  const now = Date.now();
  return () => {
    const id = uuidv4();
    const content = generateContent(rng);
    const t0 = process.hrtime.bigint();
    stmt.run(id, 1, 'markdown-text', now, now, content);
    return process.hrtime.bigint() - t0;
  };
}

// Returns an Op: () => BigInt | null (null when pool exhausted)
function createVersionOp(db, index, rng) {
  const getVersion = db.prepare('SELECT MAX(version_id) as max_v, create_time FROM entries WHERE id = ?');
  const archive = db.prepare('UPDATE entries SET is_latest = 0 WHERE id = ? AND is_latest = 1');
  const insert = db.prepare(
    'INSERT INTO entries (id, version_id, entry_type, create_time, modify_time, is_latest, content) VALUES (?, ?, ?, ?, ?, 1, ?)'
  );
  const doVersion = db.transaction((id, content, now) => {
    const row = getVersion.get(id);
    if (!row || !row.max_v) return false;
    archive.run(id);
    insert.run(id, row.max_v + 1, 'markdown-text', row.create_time, now, content);
    return true;
  });

  const pool = [...index];
  const now = Date.now();
  return () => {
    if (pool.length === 0) return null;
    const pick = Math.floor(rng() * pool.length);
    const entry = pool.splice(pick, 1)[0];
    const content = generateContent(rng);
    const t0 = process.hrtime.bigint();
    doVersion(entry.id, content, now);
    return process.hrtime.bigint() - t0;
  };
}

module.exports = { readRandomOp, readDayOp, createEntryOp, createVersionOp };
