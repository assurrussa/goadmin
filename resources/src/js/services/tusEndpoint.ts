export const resolveTusEndpoint = (uploadUrl?: string): string => {
  let base = (uploadUrl ?? '').trim().replace(/\/$/, '')
  const explicitTransport =
    base.includes('/files') || base.includes('/uploads') || base.includes('/tus')
  if (!base || !explicitTransport) {
    base = '/files'
  }

  const root = base.endsWith('/rich-text') ? base.slice(0, -'/rich-text'.length) : base
  return root.endsWith('/tus') ? root : `${root}/tus`
}
