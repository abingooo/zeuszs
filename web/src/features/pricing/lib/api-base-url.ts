/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { SystemStatus } from '@/features/auth/types'

function normalizeBaseUrl(value: unknown): string {
  if (typeof value !== 'string') return ''
  return value.trim().replace(/\/+$/, '')
}

/**
 * Resolve the base URL shown in generated API examples.
 *
 * API info entries are the public API addresses intended for callers. The
 * configured server address remains the compatibility fallback for instances
 * that have not configured API info yet.
 */
export function resolveApiBaseUrl(
  apiInfoUrl: string | undefined,
  status: SystemStatus | null,
  fallbackOrigin = typeof window !== 'undefined'
    ? window.location.origin
    : undefined
): string {
  const configuredApiUrl = normalizeBaseUrl(apiInfoUrl)
  if (configuredApiUrl) return configuredApiUrl

  const statusRecord = status as Record<string, unknown> | null
  const statusData = status?.data as Record<string, unknown> | undefined
  const serverAddressCandidates = [
    statusRecord?.server_address,
    statusRecord?.serverAddress,
    statusData?.server_address,
    statusData?.serverAddress,
  ]
  for (const candidate of serverAddressCandidates) {
    const serverAddress = normalizeBaseUrl(candidate)
    if (serverAddress) return serverAddress
  }

  return normalizeBaseUrl(fallbackOrigin) || 'https://api.example.com'
}
