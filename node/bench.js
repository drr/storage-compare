#!/usr/bin/env node
'use strict';

const path = require('path');
const Database = require('better-sqlite3');
const { loadIndex, buildDayPool, runAdaptive, printTable, saveCSV } = require('./runner');
const sqliteOps = require('./ops/sqlite');

const args = process.argv.slice(2);
function getArg(name, def) {
  const i = args.indexOf('--' + name);
  return i >= 0 ? args[i + 1] : def;
}
function hasFlag(name) {
  return args.includes('--' + name);
}

const dataDir    = getArg('data-dir',     path.join(__dirname, '..', 'data'));
const resultsDir = getArg('results-dir',  path.join(__dirname, '..', 'results'));
const READ_RANDOM    = parseInt(getArg('read-random',    '500'));
const READ_DAY       = parseInt(getArg('read-day',       '100'));
const CREATE_ENTRY   = parseInt(getArg('create-entry',   '200'));
const CREATE_VERSION = parseInt(getArg('create-version', '100'));
const FTS_SEARCH     = parseInt(getArg('fts-search',     '100'));
const PRECISION      = parseFloat(getArg('precision',    '0.05'));
const MAX_FACTOR     = parseInt(getArg('max-factor',     '10'));
const FTS            = hasFlag('fts');

// Mulberry32 seeded RNG
function mkRng(seed) {
  let s = seed >>> 0;
  return function() {
    s |= 0; s = s + 0x6D2B79F5 | 0;
    let t = Math.imul(s ^ s >>> 15, 1 | s);
    t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
    return ((t ^ t >>> 14) >>> 0) / 4294967296;
  };
}
const rng = mkRng(Date.now());

const index = loadIndex(dataDir);
if (!index.length) { console.error('index.json is empty — run generate first'); process.exit(1); }
console.log(`Loaded ${index.length} entries from index.`);
console.log(`Precision target: ${(PRECISION * 100).toFixed(0)}% relative CI for median | max-factor: ${MAX_FACTOR}x\n`);

const dayPool = buildDayPool(index, 10);
if (!dayPool.size) { console.error('No days with >=10 entries'); process.exit(1); }

const db = new Database(path.join(dataDir, 'sqlite', 'notes.db'));
db.pragma('journal_mode = WAL');

const results = [];

function run(backend, operation, minN, op) {
  const { timings, perUnit, converged } = runAdaptive(op, minN, PRECISION, minN * MAX_FACTOR);
  results.push({ backend, operation, timings, perUnit, converged });
  saveCSV(resultsDir, backend, operation, timings);
}

console.log('=== SQLite ===');
run('sqlite', 'read_random',    READ_RANDOM,    sqliteOps.readRandomOp(db, index, rng));
run('sqlite', 'read_day',       READ_DAY,       sqliteOps.readDayOp(db, dayPool, rng));
run('sqlite', 'create_entry',   CREATE_ENTRY,   sqliteOps.createEntryOp(db, rng));
run('sqlite', 'create_version', CREATE_VERSION, sqliteOps.createVersionOp(db, index, rng));

if (FTS) {
  const ftsDb = new Database(path.join(dataDir, 'sqlite-fts', 'notes.db'));
  ftsDb.pragma('journal_mode = WAL');
  console.log('=== SQLite-FTS ===');
  run('sqlite-fts', 'read_random',    READ_RANDOM,    sqliteOps.readRandomOp(ftsDb, index, rng));
  run('sqlite-fts', 'read_day',       READ_DAY,       sqliteOps.readDayOp(ftsDb, dayPool, rng));
  run('sqlite-fts', 'create_entry',   CREATE_ENTRY,   sqliteOps.createEntryOp(ftsDb, rng));
  run('sqlite-fts', 'create_version', CREATE_VERSION, sqliteOps.createVersionOp(ftsDb, index, rng));
  run('sqlite-fts', 'fts_search',     FTS_SEARCH,     sqliteOps.ftsSearchOp(ftsDb, rng));
  ftsDb.close();
}

db.close();
console.log('');
printTable(results, index.length);
