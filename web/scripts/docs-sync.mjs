// Sync contract files from the core runtime repository (AGENTFLOW_DIR or ../agentflow)
// into docs/ with normalized LF line endings. Runs cleanly on Windows, macOS, and Linux.
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

function runtimeFlag(argv) {
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === '--runtime' && argv[i + 1]) return argv[i + 1]
    if (argv[i].startsWith('--runtime=')) return argv[i].slice('--runtime='.length)
  }
  return ''
}

const scriptDir = path.dirname(fileURLToPath(import.meta.url))
const studioRoot = path.resolve(scriptDir, '..', '..')
const runtimeRoot = path.resolve(studioRoot, runtimeFlag(process.argv.slice(2)) || process.env.AGENTFLOW_DIR || '../agentflow')

const PAIRS = [
  ['api/openapi/users.yaml', 'docs/openapi/users.yaml'],
  ['api/lua/af.d.lua', 'docs/lua/af.d.lua'],
]

for (const [srcRel, dstRel] of PAIRS) {
  const src = path.join(runtimeRoot, srcRel)
  const dst = path.join(studioRoot, dstRel)
  if (!fs.existsSync(src)) {
    console.error(`missing ${src}`)
    process.exit(1)
  }
  const text = fs.readFileSync(src, 'utf8').replace(/\r\n?/g, '\n')
  fs.mkdirSync(path.dirname(dst), { recursive: true })
  fs.writeFileSync(dst, text, 'utf8')
  console.log(`synced ${dstRel} <- ${src}`)
}
