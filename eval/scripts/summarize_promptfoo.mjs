#!/usr/bin/env node
/**
 * Normalize Promptfoo output into workshop metrics JSON.
 * Promptfoo scores are not a compliance verdict.
 */
import fs from 'node:fs';
import path from 'node:path';

const [,, inputPath, outputPath, mode = 'offline'] = process.argv;
if (!inputPath || !outputPath) {
  console.error('usage: summarize_promptfoo.js <results.json> <out.json> [mode]');
  process.exit(2);
}

const raw = JSON.parse(fs.readFileSync(inputPath, 'utf8'));
const results = raw.results || raw;
const stats = results.stats || {};
const successes = Number(stats.successes || 0);
const failures = Number(stats.failures || 0);
const total = successes + failures;
const summary = {
  mode,
  pass_rate: total ? successes / total : 0,
  results: { stats: { successes, failures } },
  note: 'Promptfoo scores are not a compliance verdict',
};
fs.mkdirSync(path.dirname(path.resolve(outputPath)), { recursive: true });
fs.writeFileSync(outputPath, JSON.stringify(summary, null, 2) + '\n');
