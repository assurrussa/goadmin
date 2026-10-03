// Export is a file transfer, not an Inertia page visit. Use the grid's URL state
// and leave page size/selection out of the matching-row export contract.
export async function downloadUserExport(
  apiUrl: string,
  pageUrl: string,
  signal: AbortSignal,
): Promise<boolean> {
  const current = new URL(pageUrl)
  const target = new URL(`${apiUrl.replace(/\/$/, '')}/export`, current.origin)
  if (target.origin !== current.origin || target.username || target.password) {
    throw new Error('Недопустимый адрес экспорта.')
  }
  target.search = current.search
  target.searchParams.delete('page')
  target.searchParams.delete('limit')
  target.hash = ''
  const response = await fetch(target.href, {
    credentials: 'same-origin',
    headers: { Accept: 'text/csv' },
    signal,
  })
  if (
    !response.ok ||
    response.headers.get('content-type')?.split(';')[0]?.trim().toLowerCase() !== 'text/csv'
  ) {
    throw new Error('Не удалось экспортировать пользователей. Проверьте сеанс и повторите попытку.')
  }
  const blob = await response.blob()
  signal.throwIfAborted()
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  const filename = response.headers
    .get('content-disposition')
    ?.match(/filename="(users-\d{8}\.csv)"/)?.[1]
  link.download = filename || 'users.csv'
  document.body.append(link)
  try {
    link.click()
  } finally {
    link.remove()
    setTimeout(() => URL.revokeObjectURL(url), 0)
  }
  return response.headers.get('x-goadmin-export-truncated') === 'true'
}
