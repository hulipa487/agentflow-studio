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
  fs.rmSync(path.join(desktopDir, 'packaging', 'windows', 'app.ico'), { force: true })
  fs.rmSync(path.join(desktopDir, 'packaging', 'dist'), { recursive: true, force: true })
  fs.rmSync(desktopDist, { recursive: true, force: true })
  ensureGitkeep()
}

// Pack winres PNGs into a standard multi-resolution PNG-in-ICO container.
function makeIco(outPath = path.join(desktopDir, 'packaging', 'windows', 'app.ico')) {
  const sizes = [16, 24, 32, 48, 64, 128, 256]
  const images = sizes.map((size) => {
    const file = path.join(desktopDir, 'winres', `icon${size}.png`)
    if (!fs.existsSync(file)) {
      console.error(`missing icon frame: ${file}`)
      process.exit(1)
    }
    return { size, data: fs.readFileSync(file) }
  })

  const headerSize = 6 + images.length * 16
  let offset = headerSize
  const totalBytes = images.reduce((acc, im) => acc + im.data.length, headerSize)
  const buf = Buffer.alloc(totalBytes)
  buf.writeUInt16LE(0, 0) // reserved
  buf.writeUInt16LE(1, 2) // icon type
  buf.writeUInt16LE(images.length, 4)

  images.forEach((im, idx) => {
    const dir = 6 + idx * 16
    buf.writeUInt8(im.size >= 256 ? 0 : im.size, dir)
    buf.writeUInt8(im.size >= 256 ? 0 : im.size, dir + 1)
    buf.writeUInt8(0, dir + 2)
    buf.writeUInt8(0, dir + 3)
    buf.writeUInt16LE(1, dir + 4)
    buf.writeUInt16LE(32, dir + 6)
    buf.writeUInt32LE(im.data.length, dir + 8)
    buf.writeUInt32LE(offset, dir + 12)
    im.data.copy(buf, offset)
    offset += im.data.length
  })

  fs.mkdirSync(path.dirname(outPath), { recursive: true })
  fs.writeFileSync(outPath, buf)
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
  case 'make-ico':
    makeIco(arg || undefined)
    break
  case 'build-windows':
    buildWindows(arg)
    break
  default:
    console.error('usage: node desktop-assets.mjs <sync|clean|make-ico [out]|build-windows [version]>')
    process.exit(2)
}
