#!/usr/bin/env node
// Copies the documentation set that ships with the desktop Studio into
// src/docs/content, so the web app bundles the same guides and works offline.
//
//   npm run docs:sync                 # from ../agentflow-studio/docs
//   STUDIO_DIR=/path/to/agentflow-studio npm run docs:sync
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const studio = path.resolve(process.env.STUDIO_DIR || path.join(here, '..', '..', 'agentflow-studio'))
const src = path.join(studio, 'docs')
const dst = path.join(here, '..', 'src', 'docs', 'content')

if (!fs.existsSync(src)) {
  console.error(`missing ${src}; set STUDIO_DIR to an agentflow-studio checkout`)
  process.exit(1)
}
fs.rmSync(dst, { recursive: true, force: true })
let n = 0
const walk = (dir) => {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const from = path.join(dir, e.name)
    const to = path.join(dst, path.relative(src, from))
    if (e.isDirectory()) walk(from)
    else if (/\.(md|yaml|yml|luau|json)$/.test(e.name)) {
      fs.mkdirSync(path.dirname(to), { recursive: true })
      // Keep LF so the bundle is identical on every platform.
      fs.writeFileSync(to, fs.readFileSync(from, 'utf8').replace(/\r\n/g, '\n'))
      n++
    }
  }
}
walk(src)
console.log(`synced ${n} documentation files from ${src}`)
