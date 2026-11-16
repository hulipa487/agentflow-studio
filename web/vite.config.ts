import { fileURLToPath, URL } from 'node:url'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
// @ts-ignore -- plain ESM module shared with server/serve.mjs
import { createProxyHandler } from './server/proxy.mjs'

// The local proxy rides on Vite's own server in dev and preview, so the
// browser only ever talks to this origin: no CORS entry on the runtime, and
// Unix sockets / named pipes become reachable.
function agentflowProxy(): Plugin {
  const handler = createProxyHandler()
  const mount = (server: { middlewares: { use: (fn: any) => void } }) => {
    server.middlewares.use((req: any, res: any, next: () => void) => {
      if (!handler(req, res)) next()
    })
  }
  return { name: 'agentflow-proxy', configureServer: mount, configurePreviewServer: mount }
}

export default defineConfig({
  // Relative asset URLs: the build can be served from any path.
  base: './',
  plugins: [react(), tailwindcss(), agentflowProxy()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  // The app and its proxy are same-origin, so the dev server sends no CORS
  // headers at all: another site cannot read the proxy through this origin.
  // fs.allow reaches one level up for the shared ../docs directory, which the
  // app bundles (src/docs/library.ts).
  server: { host: '127.0.0.1', port: 5173, strictPort: false, cors: false, fs: { allow: ['..'] } },
  preview: { host: '127.0.0.1', port: 4173, cors: false },
  build: { target: 'es2022', chunkSizeWarningLimit: 1500 },
})
