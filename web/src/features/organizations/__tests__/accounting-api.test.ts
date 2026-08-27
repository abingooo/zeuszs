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
import { afterEach, describe, expect, test } from 'vitest'

import { api } from '@/lib/api'

import {
  allocateTenantMemberQuota,
  recoverTenantMemberQuota,
} from '../tenant-api'

type ApiPost = (
  url: string,
  data?: unknown,
  config?: { headers?: Record<string, string> }
) => Promise<{ data: Record<string, unknown> }>

const apiClient = api as unknown as { post: ApiPost }
const originalPost = apiClient.post

afterEach(() => {
  apiClient.post = originalPost
})

describe('tenant organization accounting API', () => {
  test('sends a distinct bounded idempotency key for each quota transfer', async () => {
    const calls: Array<{
      url: string
      data: unknown
      idempotencyKey: string | undefined
    }> = []
    apiClient.post = async (url, data, config) => {
      calls.push({
        url,
        data,
        idempotencyKey: config?.headers?.['Idempotency-Key'],
      })
      return {
        data: {
          success: true,
          data: {
            ledger_id: 1,
            user_quota_after: 100,
            pool_quota_after: 900,
            recoverable_quota_after: 100,
            already_applied: false,
          },
        },
      }
    }

    await allocateTenantMemberQuota(7, 100)
    await recoverTenantMemberQuota(7, 50)

    expect(calls.map((call) => [call.url, call.data])).toEqual([
      ['/api/organization/members/7/allocate', { amount: 100 }],
      ['/api/organization/members/7/recover', { amount: 50 }],
    ])
    const keys = calls.map((call) => call.idempotencyKey)
    expect(
      keys.every((key) => typeof key === 'string' && key.length <= 128)
    ).toBe(true)
    expect(keys[0]).not.toBe(keys[1])
  })
})
