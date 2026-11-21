import { spawn } from 'node:child_process'
import { createInterface } from 'node:readline'

// Drive the same newline-delimited MCP stdio protocol as a desktop client.
// Each connection owns exactly one child and its in-memory device grant.
export async function connectProductEventsMCP(binary, adminURL) {
  const child = spawn(binary, ['-admin-url', adminURL], { stdio: ['pipe', 'pipe', 'inherit'] })
  const lines = createInterface({ input: child.stdout })
  const pending = new Map()
  let sequence = 0
  const exited = new Promise(resolve => child.once('close', resolve))
  let stopped
  function stop(error) {
    stopped = error
    for (const waiting of pending.values()) waiting.reject(error)
    pending.clear()
  }
  child.once('error', stop)
  child.stdin.on('error', stop)
  child.once('close', () => stop(new Error('MCP process closed')))
  lines.on('line', line => {
    let message
    try { message = JSON.parse(line) } catch {
      stop(new Error('Unexpected data on MCP stdout'))
      return
    }
    const waiting = pending.get(message.id)
    if (!waiting) return
    pending.delete(message.id)
    if (message.error) waiting.reject(new Error(`MCP protocol error ${message.error.code}`))
    else waiting.resolve(message.result)
  })
  function send(message) {
    child.stdin.write(JSON.stringify({ jsonrpc: '2.0', ...message }) + '\n')
  }
  async function request(method, params) {
    if (stopped) throw stopped
    const id = ++sequence
    const waiting = Promise.withResolvers()
    pending.set(id, waiting)
    const timer = setTimeout(() => waiting.reject(new Error(`MCP timeout: ${method}`)), 20000)
    try {
      send({ id, method, params })
      return await waiting.promise
    } finally {
      clearTimeout(timer)
      pending.delete(id)
    }
  }
  async function close() {
    child.stdin.end()
    const timer = setTimeout(() => child.kill('SIGKILL'), 3000)
    try { await exited } finally { clearTimeout(timer); lines.close() }
  }
  try {
    const ready = await request('initialize', {
      protocolVersion: '2025-11-25',
      capabilities: {},
      clientInfo: { name: 'starter-smoke', version: '1' },
    })
    if (ready.protocolVersion !== '2025-11-25') throw new Error('Unexpected MCP protocol version')
    send({ method: 'notifications/initialized' })
  } catch (error) {
    await close()
    throw error
  }
  return {
    listTools: () => request('tools/list', {}),
    callTool: (name, args) => request('tools/call', { name, arguments: args }),
    close,
  }
}
