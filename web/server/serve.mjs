#!/usr/bin/env node
// Serves the built app (dist/) and the local proxy from one loopback port.
//
//   npm run build && npm start            # http://127.0.0.1:4173
//   PORT=9000 npm start
//   HOST=0.0.0.0 ALLOW_HOSTS=studio.lan npm start   # behind your own TLS/auth
import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { createProxyHandler } from './proxy.mjs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'dist')
const host = process.env.HOST || '127.0.0.1'
const port = Number(process.env.PORT || 4173)
const allowHosts = (process.env.ALLOW_HOSTS || '').split(',').map((s) => s.trim()).filter(Boolean)

if (!fs.existsSync(path.join(root, 'index.html'))) {
  console.error('dist/ is missing: run `npm run build` first')
  process.exit(1)
}

const TYPES = {
  '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8', '.svg': 'image/svg+xml', '.json': 'application/json',
  '.woff2': 'font/woff2', '.woff': 'font/woff', '.png': 'image/png', '.ico': 'image/x-icon',
  '.map': 'application/json', '.txt': 'text/plain; charset=utf-8',
}

const proxy = createProxyHandler({ allowHosts })

const server = http.createServer((req, res) => {
  if (proxy(req, res)) return
  if (req.method !== 'GET' && req.method !== 'HEAD') {
    res.writeHead(405).end()
    return
  }
  let rel = decodeURIComponent((req.url || '/').split('?')[0])
  if (rel.endsWith('/')) rel += 'index.html'
  let file = path.join(root, path.normalize(rel))
  if (!file.startsWith(root)) {
    res.writeHead(403).end()
    return
  }
  // The app uses hash routes, so anything that is not a file is the shell.
  if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) file = path.join(root, 'index.html')
  const ext = path.extname(file)
  res.writeHead(200, {
    'Content-Type': TYPES[ext] || 'application/octet-stream',
    'Cache-Control': file.includes(`${path.sep}assets${path.sep}`) ? 'public, max-age=31536000, immutable' : 'no-store',
    'X-Content-Type-Options': 'nosniff',
    'Referrer-Policy': 'no-referrer',
  })
  if (req.method === 'HEAD') return res.end()
  fs.createReadStream(file).pipe(res)
})

server.listen(port, host, () => {
  console.log(`AgentFlow Studio (web) on http://${host === '0.0.0.0' ? '127.0.0.1' : host}:${port}`)
  if (host !== '127.0.0.1' && host !== 'localhost') {
    console.log('warning: listening beyond loopback. The proxy forwards admin tokens; put TLS and auth in front.')
  }
})
