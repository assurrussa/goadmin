export function terminalUpload(message) {
  try {
    const value = typeof message === 'string' || Buffer.isBuffer(message)
      ? JSON.parse(message.toString()) : message
    return value?.eventType === 'files.upload.status' &&
      ['completed', 'failed', 'error'].includes(value.status) ? value : null
  } catch { return null }
}

export function taskStatus(payload) {
  const file = payload.file ?? payload.task?.file
  const status = payload.status || payload.task?.status || file?.status || 'queued'
  return file?.url && ['queued', 'processing'].includes(status) ? 'completed' : status
}

export function deferred() {
  let resolve
  const promise = new Promise((done) => { resolve = done })
  return { promise, resolve }
}

export async function deadline(promise, description, milliseconds = 30000) {
  let timer
  try {
    return await Promise.race([promise, new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error(`Timeout: ${description}`)), milliseconds)
    })])
  } finally { clearTimeout(timer) }
}
