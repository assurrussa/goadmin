import { createServer, type Server } from 'node:http'
import type { AddressInfo } from 'node:net'
import axios from 'axios'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { uploadFiles } from '../fileUploadService'
let server: Server | undefined
const originalAdapter = axios.defaults.adapter
afterEach(async () => {
  axios.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
  if (server) {
    server.closeAllConnections()
    await new Promise<void>((resolve) => server!.close(() => resolve()))
    server = undefined
  }
})
describe('real XHR cancellation', () => {
  it.each(['timeout', 'cancel'])('aborts a pending network request on %s', async (mode) => {
    let received!: () => void
    const pendingRequest = new Promise<void>((resolve) => {
      received = resolve
    })
    server = createServer((req, res) => {
      res.setHeader('Access-Control-Allow-Origin', '*')
      res.setHeader('Access-Control-Allow-Headers', '*')
      res.setHeader('Access-Control-Allow-Methods', 'POST,OPTIONS')
      if (req.method === 'OPTIONS') {
        res.writeHead(204)
        res.end()
        return
      }
      req.resume()
      received()
    })
    await new Promise<void>((resolve) => server!.listen(0, '127.0.0.1', resolve))
    const port = (server.address() as AddressInfo).port
    axios.defaults.adapter = 'xhr'
    const abort = vi.spyOn(XMLHttpRequest.prototype, 'abort')
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const controller = new AbortController()
    const result = uploadFiles({
      file: new File(['audio'], 'track.mp3'),
      entityType: 'meditation',
      entityId: 1,
      uploadUrl: `http://127.0.0.1:${port}/files/tus`,
      requestTimeoutMs: 1000,
      signal: controller.signal,
    })
    await pendingRequest
    if (mode === 'cancel') controller.abort()
    expect((await result).error).toContain(mode === 'cancel' ? 'cancelled' : 'timed out')
    expect(abort).toHaveBeenCalledTimes(1)
  })
})
