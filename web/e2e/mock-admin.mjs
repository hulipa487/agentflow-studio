#!/usr/bin/env node
// Deterministic mock of AgentFlow's /admin/api for Playwright E2E tests.
import http from 'node:http'

const host = '127.0.0.1'
const port = Number(process.env.MOCK_ADMIN_PORT || 18089)
const expectedToken = process.env.MOCK_ADMIN_TOKEN || 'test-admin-token'

const SAMPLE_LOOP = `-- af-prelude-version: 1
local msg = af.input()
local res, err = af.llm.chat({
  messages = {
    { role = "user", content = msg.text or "" },
  },
})
if err then
  af.send("Error: " .. tostring(err))
  return
end
af.send(res.text or "")
`

const SAMPLE_YAML = `model: gpt-4o-mini
safety: default
memory: conversational
capabilities:
  - llm
`

const threadsByAgent = new Map()
const tracesByThread = new Map()

const safetyState = {
  mtime: 1,
  builtin_filters: ['prompt_injection', 'pii_redact', 'steady_directive', 'support_offer'],
  profiles: [
    {
      name: 'default',
      filters: ['prompt_injection'],
      agents: ['helper'],
      spawn_profiles: [],
      live: true,
    },
  ],
  classifiers: [],
  lua_filters: [],
  models: ['gpt-4o-mini'],
  prompts: [],
}

function safetyPayload(savedNote = '') {
  const profiles = safetyState.profiles.map((p) => {
    const filters = p.filters || []
    return {
      name: p.name,
      filters,
      agents: p.agents || [],
      spawn_profiles: p.spawn_profiles || [],
      classifiers: filters.filter((f) => f.startsWith('classifier:')).map((f) => f.slice('classifier:'.length)),
      lua_filters: filters.filter((f) => f.startsWith('lua:')).map((f) => f.slice('lua:'.length)),
      live: Boolean(p.live),
    }
  })
  const usedBy = (prefix, name) => profiles.filter((p) => p.filters.includes(prefix + name)).map((p) => p.name)
  const classifiers = safetyState.classifiers.map((c) => ({
    ...c,
    used_by: usedBy('classifier:', c.name),
  }))
  const lua_filters = safetyState.lua_filters.map((lf) => ({
    ...lf,
    used_by: usedBy('lua:', lf.name),
  }))
  return {
    ok: true,
    mode: 'configdir',
    path: '/mock/safety.yaml',
    editable: true,
    mtime: safetyState.mtime,
    restart_required: safetyState.mtime > 1,
    saved_note: savedNote,
    builtin_filters: safetyState.builtin_filters,
    profiles,
    classifiers,
    lua_filters,
    models: safetyState.models,
    prompts: safetyState.prompts,
  }
}

function readBody(req) {
  return new Promise((resolve) => {
    const chunks = []
    req.on('data', (c) => chunks.push(c))
    req.on('end', () => {
      try {
        resolve(JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}'))
      } catch {
        resolve({})
      }
    })
  })
}

function sendJSON(res, status, data) {
  res.writeHead(status, {
    'Content-Type': 'application/json',
    'Cache-Control': 'no-store',
  })
  res.end(JSON.stringify(data))
}

function nowISO(offsetMs = 0) {
  return new Date(Date.now() + offsetMs).toISOString()
}

function makeChatTrace(agent, thread, text) {
  const session = `sess-${thread}`
  const turn = `turn-${Date.now()}`
  return [
    {
      seq: 1,
      ts: nowISO(-30),
      agent,
      session,
      thread,
      turn,
      kind: 'turn.start',
      ok: true,
      text,
    },
    {
      seq: 2,
      ts: nowISO(-22),
      agent,
      session,
      thread,
      turn,
      kind: 'op.call',
      op: 'af.llm.chat',
      model: 'gpt-4o-mini',
      ok: true,
      duration_ms: 42,
      tokens_in: 18,
      tokens_out: 14,
      messages: 1,
    },
    {
      seq: 3,
      ts: nowISO(-5),
      agent,
      session,
      thread,
      turn,
      kind: 'reply',
      ok: true,
      text: `Echo (${agent}): ${text}`,
    },
    {
      seq: 4,
      ts: nowISO(0),
      agent,
      session,
      thread,
      turn,
      kind: 'turn.end',
      ok: true,
      duration_ms: 48,
      tokens_in: 18,
      tokens_out: 14,
    },
  ]
}

const server = http.createServer(async (req, res) => {
  const u = new URL(req.url || '/', `http://${host}:${port}`)
  const p = u.pathname

  if (p === '/healthz') {
    res.writeHead(200, { 'Content-Type': 'text/plain' }).end('ok')
    return
  }

  if (p.startsWith('/admin/api/')) {
    const auth = req.headers.authorization || ''
    if (auth !== `Bearer ${expectedToken}`) {
      sendJSON(res, 401, { ok: false, error: 'invalid admin token' })
      return
    }
  }

  if (p === '/admin/api/state' && req.method === 'GET') {
    sendJSON(res, 200, {
      ok: true,
      version: '0.1.0-mock',
      started_at: Math.floor(Date.now() / 1000) - 120,
      uptime_s: 120,
      config_path: '/mock/agentflow.yaml',
      config_epoch: 'epoch-1',
      instance: 'mock-inst-1',
      region: 'local',
      topology: 'single',
      zone: 'dev',
      instances: [
        {
          owner: 'mock-inst-1',
          region: 'local',
          zone: 'dev',
          version: '0.1.0-mock',
          topology: 'single',
          config_epoch: 'epoch-1',
          config_epoch_differs: false,
          started_at: Math.floor(Date.now() / 1000) - 120,
          last_seen: Math.floor(Date.now() / 1000),
          self: true,
        },
      ],
      agents: [{ name: 'helper', model: 'gpt-4o-mini', loop: 'loops/helper.lua', persistent: false }],
      channels: [],
      sessions: [],
      sessions_active: 0,
      sessions_idle: 0,
      credentials_enabled: true,
    })
    return
  }

  if (p === '/admin/api/metrics' && req.method === 'GET') {
    const t0 = Math.floor(Date.now() / 1000) - 60
    sendJSON(res, 200, {
      ok: true,
      series: {
        agentflow_llm_calls: [{ at: t0, v: 2 }, { at: t0 + 60, v: 3 }],
        agentflow_llm_tokens: [{ at: t0, v: 64 }, { at: t0 + 60, v: 96 }],
        agentflow_ingress_total: [{ at: t0, v: 2 }, { at: t0 + 60, v: 3 }],
        agentflow_safety_drops: [{ at: t0, v: 0 }, { at: t0 + 60, v: 0 }],
      },
      latest: {
        agentflow_llm_calls: 3,
        agentflow_llm_tokens: 96,
        agentflow_ingress_total: 3,
        agentflow_safety_drops: 0,
      },
    })
    return
  }

  if (p === '/admin/api/studio/catalog' && req.method === 'GET') {
    sendJSON(res, 200, {
      catalog: {
        models: ['gpt-4o-mini', 'claude-3-5-sonnet'],
        tools: ['web_search'],
        capabilities: ['llm', 'memory', 'tools', 'exec', 'kv'],
        default_capabilities: ['llm', 'memory'],
        safety_profiles: ['default', 'strict', 'none'],
        memory_profiles: ['conversational'],
        prompt_keys: ['system'],
        prelude_version: 1,
        templates: [],
      },
    })
    return
  }

  if (p === '/admin/api/studio/agents' && req.method === 'GET') {
    sendJSON(res, 200, {
      agents: [
        {
          name: 'helper',
          model: 'gpt-4o-mini',
          loop: 'loops/helper.lua',
          spawn: false,
          persistent: false,
          capabilities: ['llm'],
          persisted: true,
          sessions: 0,
        },
      ],
    })
    return
  }

  if (p === '/admin/api/studio/agents/helper' && req.method === 'GET') {
    sendJSON(res, 200, {
      ok: true,
      name: 'helper',
      source: SAMPLE_YAML,
      agent_yaml: SAMPLE_YAML,
      agent: {
        loop: 'loops/helper.lua',
        model: 'gpt-4o-mini',
        history_budget: 20,
        safety: 'default',
        memory: 'conversational',
        skills: [],
        capabilities: ['llm'],
        can_contact: [],
        persistent: false,
        credentials: [],
      },
      loop: SAMPLE_LOOP,
      instructions: 'Be concise and helpful.',
      files: {
        mode: 'directory',
        profile_path: '/mock/agents/helper.yaml',
        loop_path: '/mock/loops/helper.lua',
        instructions_path: '/mock/prompts/helper.md',
        managed: true,
        mtime: 1,
      },
      persisted: true,
      spawn: false,
      persistent: false,
      model: 'gpt-4o-mini',
      capabilities: ['llm'],
      skills: [],
      sessions: 0,
    })
    return
  }

  if (p === '/admin/api/studio/threads' && req.method === 'GET') {
    const agent = u.searchParams.get('agent') || 'helper'
    sendJSON(res, 200, { threads: threadsByAgent.get(agent) || [] })
    return
  }

  if (p.startsWith('/admin/api/studio/trace/') && req.method === 'GET') {
    const [, , , , agent, thread] = p.split('/')
    const key = `${decodeURIComponent(agent)}/${decodeURIComponent(thread)}`
    sendJSON(res, 200, { events: tracesByThread.get(key) || [] })
    return
  }

  if (p === '/admin/api/studio/chat' && req.method === 'POST') {
    const body = await readBody(req)
    const agent = body.agent || 'helper'
    const thread = body.thread || `th-${Date.now()}`
    const text = body.text || ''
    const events = makeChatTrace(agent, thread, text)
    const key = `${agent}/${thread}`
    tracesByThread.set(key, events)
    const existing = threadsByAgent.get(agent) || []
    threadsByAgent.set(agent, [
      { agent, thread, session: `sess-${thread}`, last: nowISO(), events: events.length },
      ...existing.filter((r) => r.thread !== thread),
    ])

    res.writeHead(200, {
      'Content-Type': 'text/event-stream; charset=utf-8',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive',
    })
    for (const ev of events) {
      res.write(`event: trace\ndata: ${JSON.stringify({ type: 'trace', trace: ev })}\n\n`)
    }
    const reply = { text: `Echo (${agent}): ${text}`, status: 'delivered' }
    res.write(`event: reply\ndata: ${JSON.stringify({ type: 'reply', reply })}\n\n`)
    res.write(
      `event: done\ndata: ${JSON.stringify({
        type: 'done',
        done: { thread, session: `sess-${thread}`, replies: [reply], events: events.length, timed_out: false },
      })}\n\n`,
    )
    res.end()
    return
  }

  if (p === '/admin/api/studio/loop/test' && req.method === 'POST') {
    const body = await readBody(req)
    const agent = body.agent || 'helper'
    const events = [
      { seq: 1, ts: nowISO(-25), agent, session: 'replay-sess', kind: 'turn.start', ok: true, text: 'scenario input' },
      {
        seq: 2,
        ts: nowISO(-15),
        agent,
        session: 'replay-sess',
        kind: 'op.call',
        op: 'af.llm.chat',
        model: 'gpt-4o-mini',
        ok: true,
        duration_ms: 21,
        tokens_in: 12,
        tokens_out: 10,
      },
      { seq: 3, ts: nowISO(-5), agent, session: 'replay-sess', kind: 'reply', ok: true, text: `Replay output for ${agent}` },
      { seq: 4, ts: nowISO(0), agent, session: 'replay-sess', kind: 'turn.end', ok: true, duration_ms: 24, tokens_in: 12, tokens_out: 10 },
    ]
    sendJSON(res, 200, {
      ok: true,
      result: {
        ok: true,
        agent,
        loop: 'loops/helper.lua',
        model: 'gpt-4o-mini',
        duration_ms: 24,
        sends: [`Replay output for ${agent}`],
        ops: ['af.llm.chat'],
        assertions: [
          { kind: 'send_contains', index: 0, expected: 'Replay output', actual: `Replay output for ${agent}`, ok: true },
        ],
        events,
      },
    })
    return
  }

  if (p === '/admin/api/users' && req.method === 'GET') {
    sendJSON(res, 200, { day: '2026-11-22', users: [] })
    return
  }
  if (p === '/admin/api/models' && req.method === 'GET') {
    sendJSON(res, 200, { models: [], file_error: '' })
    return
  }
  if (p === '/admin/api/studio/plugins' && req.method === 'GET') {
    sendJSON(res, 200, { ok: true, plugins: [] })
    return
  }
  if (p === '/admin/api/studio/safety' && req.method === 'GET') {
    sendJSON(res, 200, safetyPayload())
    return
  }
  if (p === '/admin/api/studio/safety/validate' && req.method === 'POST') {
    const body = await readBody(req)
    const errors = []
    if (body.kind === 'lua_filter') {
      const lf = body.lua_filter || {}
      if (!lf.name) errors.push('lua filter name is required')
      if (!lf.source && !lf.file) errors.push('lua source or file is required')
      if (lf.source && /\bos\.execute\b/.test(lf.source)) errors.push('forbidden global os')
    } else if (body.kind === 'profile') {
      const pr = body.profile || {}
      if (!pr.name) errors.push('profile name is required')
    }
    sendJSON(res, 200, { ok: errors.length === 0, errors })
    return
  }
  if (p === '/admin/api/studio/safety/lua-filters' && req.method === 'POST') {
    const body = await readBody(req)
    const lf = body.lua_filter || {}
    safetyState.lua_filters = [...safetyState.lua_filters.filter((x) => x.name !== lf.name), { ...lf, live: false }]
    safetyState.mtime++
    sendJSON(res, 200, safetyPayload('Saved to /mock/safety.yaml (restart to apply)'))
    return
  }
  if (p.startsWith('/admin/api/studio/safety/lua-filters/') && req.method === 'PUT') {
    const name = decodeURIComponent(p.slice('/admin/api/studio/safety/lua-filters/'.length))
    const body = await readBody(req)
    const lf = { ...(body.lua_filter || {}), name, live: false }
    safetyState.lua_filters = safetyState.lua_filters.map((x) => (x.name === name ? lf : x))
    safetyState.mtime++
    sendJSON(res, 200, safetyPayload('Saved to /mock/safety.yaml (restart to apply)'))
    return
  }
  if (p.startsWith('/admin/api/studio/safety/lua-filters/') && req.method === 'DELETE') {
    const name = decodeURIComponent(p.slice('/admin/api/studio/safety/lua-filters/'.length))
    if (safetyState.profiles.some((pr) => (pr.filters || []).includes('lua:' + name))) {
      sendJSON(res, 400, { ok: false, error: `lua filter "${name}" is still in use` })
      return
    }
    safetyState.lua_filters = safetyState.lua_filters.filter((x) => x.name !== name)
    safetyState.mtime++
    sendJSON(res, 200, safetyPayload('Lua filter deleted'))
    return
  }
  if (p === '/admin/api/studio/safety/profiles' && req.method === 'POST') {
    const body = await readBody(req)
    const pr = { ...(body.profile || {}), agents: [], spawn_profiles: [], live: false }
    safetyState.profiles = [...safetyState.profiles.filter((x) => x.name !== pr.name), pr]
    safetyState.mtime++
    sendJSON(res, 200, safetyPayload('Saved to /mock/safety.yaml (restart to apply)'))
    return
  }
  if (p.startsWith('/admin/api/studio/safety/profiles/') && req.method === 'PUT') {
    const name = decodeURIComponent(p.slice('/admin/api/studio/safety/profiles/'.length))
    const body = await readBody(req)
    const prev = safetyState.profiles.find((x) => x.name === name)
    const pr = { ...(prev || {}), ...(body.profile || {}), name, live: false }
    safetyState.profiles = safetyState.profiles.map((x) => (x.name === name ? pr : x))
    safetyState.mtime++
    sendJSON(res, 200, safetyPayload('Saved to /mock/safety.yaml (restart to apply)'))
    return
  }
  if (p.startsWith('/admin/api/studio/safety/profiles/') && req.method === 'DELETE') {
    const name = decodeURIComponent(p.slice('/admin/api/studio/safety/profiles/'.length))
    const prev = safetyState.profiles.find((x) => x.name === name)
    if (prev && ((prev.agents || []).length > 0 || (prev.spawn_profiles || []).length > 0)) {
      sendJSON(res, 400, { ok: false, error: `profile "${name}" is still in use` })
      return
    }
    safetyState.profiles = safetyState.profiles.filter((x) => x.name !== name)
    safetyState.mtime++
    sendJSON(res, 200, safetyPayload('Profile deleted'))
    return
  }

  sendJSON(res, 404, { ok: false, error: `unhandled mock route ${req.method} ${p}` })
})

server.listen(port, host, () => {
  console.log(`Mock admin API listening on http://${host}:${port}`)
})
