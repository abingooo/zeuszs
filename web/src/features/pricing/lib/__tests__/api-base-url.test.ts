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
import { describe, expect, test } from 'vitest'

import { resolveApiBaseUrl } from '../api-base-url'

describe('pricing API base URL resolution', () => {
  test('prefers the configured API info address over ServerAddress', () => {
    expect(
      resolveApiBaseUrl(
        '  https://api.zeuszs.ai/// ',
        { server_address: 'https://zeuszs.ai' },
        'https://fallback.example.com'
      )
    ).toBe('https://api.zeuszs.ai')
  })

  test('falls back to the top-level ServerAddress when API info is absent', () => {
    expect(
      resolveApiBaseUrl(undefined, { server_address: 'https://zeuszs.ai/' }, '')
    ).toBe('https://zeuszs.ai')
  })

  test('supports the nested ServerAddress status shape', () => {
    expect(
      resolveApiBaseUrl(
        '',
        { data: { serverAddress: 'https://nested.example.com/' } },
        ''
      )
    ).toBe('https://nested.example.com')
  })

  test('skips an empty top-level address before checking nested status data', () => {
    expect(
      resolveApiBaseUrl(
        undefined,
        {
          server_address: '   ',
          data: { server_address: 'https://nested.example.com/' },
        },
        ''
      )
    ).toBe('https://nested.example.com')
  })

  test('falls back to the current origin when no configured address exists', () => {
    expect(
      resolveApiBaseUrl(undefined, null, 'https://current.example.com/')
    ).toBe('https://current.example.com')
  })

  test('uses the example URL during server-side rendering without an origin', () => {
    expect(resolveApiBaseUrl(undefined, null, '')).toBe(
      'https://api.example.com'
    )
  })
})
