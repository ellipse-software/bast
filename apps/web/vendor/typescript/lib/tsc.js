#!/usr/bin/env node

import { createRequire } from "node:module";
import path from "node:path";
import { pathToFileURL } from "node:url";

const require = createRequire(import.meta.url);
const nativePackage = require.resolve("typescript7/package.json");
await import(
	pathToFileURL(path.join(path.dirname(nativePackage), "lib", "tsc.js")).href
);
