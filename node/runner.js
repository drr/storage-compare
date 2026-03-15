'use strict';

const fs = require('fs');
const path = require('path');

function loadIndex(dataDir) {
  const data = fs.readFileSync(path.join(dataDir, 'index.json'), 'utf8');
  return JSON.parse(data);
}

function buildDayPool(index, minEntries = 10) {
  const m = new Map();
  for (const e of index) {
    if (!m.has(e.day)) m.set(e.day, []);
    m.get(e.day).push(e);
  }
  const pool = new Map();
  for (const [day, entries] of m) {
    if (entries.length >= minEntries) pool.set(day, entries);
  }
  return pool;
}

// 95% CI for the median via order statistics.
// Returns the relative CI width (CI_width / median).
// Returns 1.0 if fewer than 30 samples.
function medianCIWidth(sorted) {
  const n = sorted.length;
  if (n < 30) return 1.0;
  const medIdx = Math.floor(n / 2);
  const median = sorted[medIdx];
  if (median === 0n) return 0.0;
  const k = Math.ceil(0.98 * Math.sqrt(n));
  const lo = Math.max(0, medIdx - k);
  const hi = Math.min(n - 1, medIdx + k);
  return Number(sorted[hi] - sorted[lo]) / Number(median);
}

function medianConverged(sorted, precision) {
  if (sorted.length < 30) return false;
  return medianCIWidth(sorted) < precision;
}

// Run op in rounds of batchSize until the median CI width is below precision,
// or maxN total valid samples are collected.
// op() returns a BigInt nanosecond latency, or null to signal skip (pool exhausted).
function runAdaptive(op, batchSize, precision, maxN) {
  const timings = [];
  let converged = false;

  while (timings.length < maxN) {
    const need = Math.min(batchSize, maxN - timings.length);
    let consecutiveFails = 0;
    let collected = 0;
    while (collected < need && consecutiveFails < 20) {
      const t = op();
      if (t !== null) {
        timings.push(t);
        consecutiveFails = 0;
        collected++;
      } else {
        consecutiveFails++;
      }
    }

    if (consecutiveFails >= 20) break; // op exhausted

    const sorted = [...timings].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
    if (medianConverged(sorted, precision)) {
      converged = true;
      break;
    }
  }

  if (!converged) {
    const sorted = [...timings].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
    converged = medianConverged(sorted, precision);
  }

  return { timings, converged };
}

function computeStats(timingsNs) {
  if (timingsNs.length === 0) return null;
  const sorted = [...timingsNs].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
  const n = sorted.length;
  const median = sorted[Math.floor(n / 2)];
  const p95 = sorted[Math.ceil(n * 0.95) - 1];
  const p99 = sorted[Math.ceil(n * 0.99) - 1];
  const min = sorted[0];
  const max = sorted[n - 1];
  const medianMs = Number(median) / 1e6;
  const opsSec = medianMs > 0 ? 1000 / medianMs : 0;
  const ciWidth = medianCIWidth(sorted);
  return { n, min, median, p95, p99, max, opsSec, medianMs, ciWidth };
}

function fmtDur(ns) {
  return (Number(ns) / 1e6).toFixed(2) + 'ms';
}

function fmtCI(ciWidth, converged) {
  const pct = (ciWidth * 100).toFixed(1);
  return converged ? `${pct}%` : `${pct}%!`;
}

function printTable(results, population) {
  console.log(`Runtime: node  |  Population: ${population}  |  Date: ${new Date().toISOString().replace('T', ' ').slice(0, 19)}\n`);
  const w = [10, 15, 5, 6, 7, 7, 7, 7, 7, 7];
  const hdr = ['Backend', 'Operation', 'N', 'medCI', 'Min', 'Median', 'P95', 'P99', 'Max', 'ops/sec'];
  console.log(
    hdr[0].padEnd(w[0]) + ' | ' + hdr[1].padEnd(w[1]) + ' | ' +
    hdr[2].padStart(w[2]) + ' | ' + hdr[3].padStart(w[3]) + ' | ' +
    hdr[4].padStart(w[4]) + ' | ' + hdr[5].padStart(w[5]) + ' | ' +
    hdr[6].padStart(w[6]) + ' | ' + hdr[7].padStart(w[7]) + ' | ' +
    hdr[8].padStart(w[8]) + ' | ' + hdr[9].padStart(w[9])
  );
  console.log('-'.repeat(105));
  for (const r of results) {
    const s = computeStats(r.timings);
    if (!s) continue;
    console.log(
      r.backend.padEnd(w[0]) + ' | ' + r.operation.padEnd(w[1]) + ' | ' +
      String(s.n).padStart(w[2]) + ' | ' + fmtCI(s.ciWidth, r.converged).padStart(w[3]) + ' | ' +
      fmtDur(s.min).padStart(w[4]) + ' | ' + fmtDur(s.median).padStart(w[5]) + ' | ' +
      fmtDur(s.p95).padStart(w[6]) + ' | ' + fmtDur(s.p99).padStart(w[7]) + ' | ' +
      fmtDur(s.max).padStart(w[8]) + ' | ' + Math.round(s.opsSec).toString().padStart(w[9])
    );
  }
}

function saveCSV(resultsDir, backend, operation, timingsNs) {
  const dir = path.join(resultsDir, 'node');
  fs.mkdirSync(dir, { recursive: true });
  const filePath = path.join(dir, `${backend}_${operation}_timings.csv`);
  fs.writeFileSync(filePath, timingsNs.map(t => t.toString()).join('\n') + '\n');
}

module.exports = { loadIndex, buildDayPool, runAdaptive, computeStats, printTable, saveCSV };
