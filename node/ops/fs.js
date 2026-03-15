'use strict';

const fsSync = require('fs');
const path = require('path');
const matter = require('gray-matter');
const { v4: uuidv4 } = require('uuid');

function generateContent(rng) {
  const words = ['the', 'be', 'to', 'of', 'and', 'a', 'in', 'that', 'have', 'it',
    'for', 'not', 'on', 'with', 'he', 'as', 'you', 'do', 'at', 'this',
    'but', 'his', 'by', 'from', 'they', 'we', 'say', 'her', 'she', 'or',
    'an', 'will', 'my', 'one', 'all', 'would', 'there', 'their', 'what', 'so',
    'up', 'out', 'if', 'about', 'who', 'get', 'which', 'go', 'me', 'when',
    'make', 'can', 'like', 'time', 'no', 'just', 'him', 'know', 'take'];
  const n = Math.max(10, Math.min(500, Math.round(Math.exp(3.5 + rng() * 1.5))));
  let out = '';
  for (let i = 0; i < n; i++) {
    if (i > 0 && i % 60 === 0) out += '\n\n';
    else if (i > 0) out += ' ';
    out += words[Math.floor(rng() * words.length)];
  }
  return out + '.\n';
}

function buildFrontmatter(e) {
  return `---\nid: "${e.id}"\nversion_id: ${e.version_id}\nentry_type: ${e.entry_type}\ncreate_time: "${e.create_time}"\nmodify_time: "${e.modify_time}"\n---\n\n${e.content}`;
}

function isVersionedFile(name) {
  const base = name.slice(0, -3);
  const idx = base.lastIndexOf('-v');
  if (idx < 0) return false;
  return /^\d+$/.test(base.slice(idx + 2));
}

// Returns an Op: () => { ns: BigInt, count: 1 }
function readRandomOp(fsRoot, index, rng) {
  return () => {
    const entry = index[Math.floor(rng() * index.length)];
    const filePath = path.join(fsRoot, entry.day_path, entry.day + '-' + entry.id + '.md');
    try {
      const t0 = process.hrtime.bigint();
      const data = fsSync.readFileSync(filePath, 'utf8');
      matter(data);
      return { ns: process.hrtime.bigint() - t0, count: 1 };
    } catch {
      return null;
    }
  };
}

// Returns an Op: () => { ns: BigInt, count: number }
// count is the number of latest entries read from the day directory.
function readDayOp(fsRoot, dayPool, rng) {
  const days = [...dayPool.keys()];
  const dayPathMap = new Map([...dayPool].map(([day, es]) => [day, es[0]?.day_path]));
  return () => {
    const day = days[Math.floor(rng() * days.length)];
    const dir = path.join(fsRoot, dayPathMap.get(day));
    try {
      const t0 = process.hrtime.bigint();
      const files = fsSync.readdirSync(dir);
      let count = 0;
      for (const file of files) {
        if (!file.endsWith('.md') || isVersionedFile(file)) continue;
        matter(fsSync.readFileSync(path.join(dir, file), 'utf8'));
        count++;
      }
      return { ns: process.hrtime.bigint() - t0, count: Math.max(1, count) };
    } catch {
      return null;
    }
  };
}

// Returns an Op: () => { ns: BigInt, count: 1 } (unlimited)
function createEntryOp(fsRoot, rng) {
  const now = new Date().toISOString();
  const today = now.slice(0, 10);
  const [year, month] = today.split('-');
  const dir = path.join(fsRoot, year, `${year}-${month}`, today);
  fsSync.mkdirSync(dir, { recursive: true });
  return () => {
    const id = uuidv4();
    const filePath = path.join(dir, `${today}-${id}.md`);
    const content = buildFrontmatter({
      id, version_id: 1, entry_type: 'markdown-text',
      create_time: now, modify_time: now,
      content: generateContent(rng),
    });
    const t0 = process.hrtime.bigint();
    fsSync.writeFileSync(filePath, content);
    return { ns: process.hrtime.bigint() - t0, count: 1 };
  };
}

// Returns an Op: () => { ns: BigInt, count: 1 } | null (null when pool exhausted)
function createVersionOp(fsRoot, index, rng) {
  const pool = [...index];
  const now = new Date().toISOString();
  return () => {
    if (pool.length === 0) return null;
    const pick = Math.floor(rng() * pool.length);
    const ie = pool.splice(pick, 1)[0];
    const latestPath = path.join(fsRoot, ie.day_path, ie.day + '-' + ie.id + '.md');
    try {
      const existing = fsSync.readFileSync(latestPath, 'utf8');
      const parsed = matter(existing);
      const currentVersion = parsed.data.version_id || 1;
      const archivePath = path.join(fsRoot, ie.day_path, `${ie.day}-${ie.id}-v${currentVersion}.md`);
      const newContent = buildFrontmatter({
        id: ie.id, version_id: currentVersion + 1, entry_type: 'markdown-text',
        create_time: parsed.data.create_time, modify_time: now,
        content: generateContent(rng),
      });
      const t0 = process.hrtime.bigint();
      fsSync.renameSync(latestPath, archivePath);
      fsSync.writeFileSync(latestPath, newContent);
      return { ns: process.hrtime.bigint() - t0, count: 1 };
    } catch {
      return null;
    }
  };
}

module.exports = { readRandomOp, readDayOp, createEntryOp, createVersionOp };
