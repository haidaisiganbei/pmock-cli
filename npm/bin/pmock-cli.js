#!/usr/bin/env node
"use strict";

const { execFileSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const platform = process.platform;
const arch = process.arch;
const scope = process.env.PMOCK_CLI_NPM_SCOPE || "@pmock";

const override = process.env.PMOCK_CLI_BIN;
if (override) {
  run(override);
}

const pkg = `${scope}/cli-${platform}-${arch}`;
let pkgDir;
try {
  pkgDir = path.dirname(require.resolve(`${pkg}/package.json`));
} catch {
  console.error(
    `pmock-cli: platform package "${pkg}" is not installed.\n` +
      `Supported: darwin-arm64, darwin-x64, linux-x64, linux-arm64, win32-x64.`
  );
  process.exit(1);
}

const binary = path.join(pkgDir, platform === "win32" ? "pmock-cli.exe" : "pmock-cli");
if (!fs.existsSync(binary)) {
  console.error(`pmock-cli: binary not found at ${binary}`);
  process.exit(1);
}
run(binary);

function run(bin) {
  try {
    execFileSync(bin, process.argv.slice(2), { stdio: "inherit", env: process.env });
    process.exit(0);
  } catch (err) {
    process.exit(typeof err.status === "number" ? err.status : 1);
  }
}
