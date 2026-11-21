// Cross-platform desktop asset & packaging helpers for desktop/Makefile.
// Avoids POSIX shell utilities (rm, cp, mkdir, touch) so native GNU Make
// works identically on Windows cmd.exe, macOS, and Linux.
import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = path.dirname(fileURLToPath(import.meta.url))
const studioRoot = path.resolve(scriptDir, '..', '..')
const webDist = path.join(studioRoot, 'web', 'dist')
const desktopDir = path.join(studioRoot, 'desktop')
const desktopDist = path.join(desktopDir, 'frontend', 'dist')

function ensureGitkeep() {
  fs.mkdirSync(desktopDist, { recursive: true })
  fs.writeFileSync(path.join(desktopDist, '.gitkeep'), '')
}

function sync() {
  if (!fs.existsSync(path.join(webDist, 'index.html'))) {
    console.error(`missing ${webDist}; run web build first`)
    process.exit(1)
  }
  fs.rmSync(desktopDist, { recursive: true, force: true })
  fs.cpSync(webDist, desktopDist, { recursive: true })
  ensureGitkeep()
}

function clean() {
  for (const name of ['agentflow-studio', 'agentflow-studio.exe']) {
    fs.rmSync(path.join(desktopDir, name), { force: true })
  }
  if (fs.existsSync(desktopDir)) {
    for (const entry of fs.readdirSync(desktopDir)) {
      if (/^agentflow-studio-.*\.exe$/i.test(entry)) {
        fs.rmSync(path.join(desktopDir, entry), { force: true })
      }
    }
  }
  fs.rmSync(desktopDist, { recursive: true, force: true })
  ensureGitkeep()
}

function buildWindows(version = 'dev') {
  for (const arch of ['amd64', 'arm64']) {
    execFileSync(
      'go',
      [
        'build',
        '-tags',
        'desktop,production',
        '-trimpath',
        '-ldflags',
        `-s -w -H windowsgui -X main.version=${version || 'dev'}`,
        '-o',
        `agentflow-studio-${arch}.exe`,
        '.',
      ],
      {
        cwd: desktopDir,
        stdio: 'inherit',
        env: { ...process.env, CGO_ENABLED: '0', GOOS: 'windows', GOARCH: arch },
      },
    )
  }
}

const [cmd, arg] = process.argv.slice(2)
switch (cmd) {
  case 'sync':
    sync()
    break
  case 'clean':
    clean()
    break
  case 'build-windows':
    buildWindows(arg)
    break
  default:
    console.error('usage: node desktop-assets.mjs <sync|clean|build-windows [version]>')
    process.exit(2)
}
