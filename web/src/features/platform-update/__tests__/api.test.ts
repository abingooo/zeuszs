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
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { getPlatformUpdate, triggerPlatformUpdate } from '../api'

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
  },
}))

beforeEach(() => {
  vi.clearAllMocks()
})

test('checks updates through the authenticated backend endpoint', async () => {
  vi.mocked(api.get).mockResolvedValue({ data: { success: true, data: {} } })

  await getPlatformUpdate()

  expect(api.get).toHaveBeenCalledWith('/api/zeuszs/update', {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
})

test('starts an update with an empty POST body', async () => {
  vi.mocked(api.post).mockResolvedValue({ data: { success: true, data: {} } })

  await triggerPlatformUpdate()

  expect(api.post).toHaveBeenCalledWith('/api/zeuszs/update', undefined, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
})
