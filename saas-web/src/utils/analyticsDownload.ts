import type { AnalyticsCSVDownload } from '../api/analytics'

export function downloadAnalyticsCSV(download: AnalyticsCSVDownload) {
  const url = URL.createObjectURL(download.blob)
  const link = document.createElement('a')
  link.href = url
  link.download = download.filename
  link.style.display = 'none'
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
