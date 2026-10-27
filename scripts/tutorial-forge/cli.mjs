#!/usr/bin/env node
// tutorial-forge CLI.
//
//   node scripts/tutorial-forge/cli.mjs build --config <path>
//   node scripts/tutorial-forge/cli.mjs expand-acronyms --config <path>
//
// --config defaults to ./tutorial.config.json

import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';

import { build } from './build.mjs';
import { expandAcronyms, loadAcronyms } from './transforms/acronyms.mjs';

function parseArgs(argv) {
  const out = { subcommand: null, config: 'tutorial.config.json', write: false };
  const rest = argv.slice(2);
  if (rest.length === 0 || rest[0].startsWith('-')) {
    out.subcommand = 'build';
  } else {
    out.subcommand = rest.shift();
  }
  while (rest.length) {
    const arg = rest.shift();
    if (arg === '--config' || arg === '-c') {
      out.config = rest.shift();
    } else if (arg === '--write') {
      out.write = true;
    } else if (arg === '--help' || arg === '-h') {
      out.subcommand = 'help';
    } else {
      console.error(`[tutorial-forge] unknown argument: ${arg}`);
      process.exit(2);
    }
  }
  return out;
}

function printHelp() {
  console.log(`tutorial-forge — build a static HTML tutorial package from a markdown deck.

Usage:
  node scripts/tutorial-forge/cli.mjs <subcommand> [options]

Subcommands:
  build               Render the deck to a static HTML package at config.output.
  expand-acronyms     Dry-run the acronym transform; prints the diff summary.
                      Add --write to persist the change to the deck source file.

Options:
  --config, -c PATH   Path to the config JSON (default: ./tutorial.config.json)
  --write             (expand-acronyms only) write the expanded text back to
                      config.deck in place.
  --help, -h          Show this help.

Config schema: see scripts/tutorial-forge/README.md.
`);
}

function loadConfig(configPath) {
  const abs = resolve(process.cwd(), configPath);
  if (!existsSync(abs)) {
    console.error(`[tutorial-forge] config not found: ${abs}`);
    process.exit(2);
  }
  const text = readFileSync(abs, 'utf-8');
  let config;
  try {
    config = JSON.parse(text);
  } catch (err) {
    console.error(`[tutorial-forge] config is not valid JSON: ${err.message}`);
    process.exit(2);
  }
  return { config, configDir: dirname(abs), configPath: abs };
}

async function main() {
  const args = parseArgs(process.argv);
  if (args.subcommand === 'help') {
    printHelp();
    return;
  }
  const { config, configDir } = loadConfig(args.config);

  if (args.subcommand === 'build') {
    await build(config, configDir);
    return;
  }

  if (args.subcommand === 'expand-acronyms') {
    const deckPath = resolve(configDir, config.deck);
    const original = readFileSync(deckPath, 'utf-8');
    const acronyms = loadAcronyms(config.acronyms ?? {});
    const expandCount = config.acronyms?.expandCount ?? 2;
    const { text, applied } = expandAcronyms(original, acronyms, expandCount);
    const changed = text !== original;
    if (!changed) {
      console.log('No changes needed; all acronyms already expanded or absent.');
      return;
    }
    if (args.write) {
      writeFileSync(deckPath, text);
      console.log(`Wrote expanded deck to ${deckPath}`);
    } else {
      console.log(`Would update ${deckPath} (${original.length} → ${text.length} bytes). Pass --write to persist.`);
    }
    console.log('Expansions applied (acronym → first-N-occurrences expanded):');
    for (const key of Object.keys(applied).sort()) {
      console.log(`  ${key.padEnd(8)}  ${applied[key]}`);
    }
    return;
  }

  console.error(`[tutorial-forge] unknown subcommand: ${args.subcommand}`);
  printHelp();
  process.exit(2);
}

main().catch((err) => {
  console.error(`[tutorial-forge] FATAL: ${err.message}`);
  if (err.stack) console.error(err.stack);
  process.exit(1);
});
