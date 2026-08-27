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

import { createOAuthFlow, telegramLogin, wechatLoginByCode } from '../api'
import type { TelegramAuthorization } from '../lib/telegram-login'

type ApiConfig = Record<string, unknown>
type ApiGet = (
  url: string,
  config?: ApiConfig
) => Promise<{ data: Record<string, unknown> }>
type ApiPost = (
  url: string,
  data?: unknown,
  config?: ApiConfig
) => Promise<{ data: Record<string, unknown> }>

const apiClient = api as unknown as { get: ApiGet; post: ApiPost }
const originalGet = apiClient.get
const originalPost = apiClient.post

afterEach(() => {
  apiClient.get = originalGet
  apiClient.post = originalPost
  localStorage.clear()
})

describe('organization invitation in alternative authentication APIs', () => {
  test('adds the invitation to standard and custom OAuth state payloads', async () => {
    const requests: Array<{ url: string; data: unknown; config?: ApiConfig }> =
      []
    apiClient.post = async (url, data, config) => {
      requests.push({ url, data, config })
      return { data: { success: true, data: { flow_token: 'oauth-state' } } }
    }

    await createOAuthFlow('github', 'login', '  ORG-CODE  ')
    await createOAuthFlow('corporate-sso', 'login', 'ORG-CODE')

    expect(requests).toEqual([
      {
        url: '/api/oauth/state',
        data: {
          provider: 'github',
          intent: 'login',
          aff: undefined,
          organization_invite_code: 'ORG-CODE',
        },
        config: { skipAuthRefresh: true },
      },
      {
        url: '/api/oauth/state',
        data: {
          provider: 'corporate-sso',
          intent: 'login',
          aff: undefined,
          organization_invite_code: 'ORG-CODE',
        },
        config: { skipAuthRefresh: true },
      },
    ])
  })

  test('keeps ordinary login and account binding OAuth state free of an invitation', async () => {
    const payloads: unknown[] = []
    apiClient.post = async (_url, data) => {
      payloads.push(data)
      return { data: { success: true, data: 'oauth-state' } }
    }

    await createOAuthFlow('github', 'login')
    await createOAuthFlow('github', 'bind')

    for (const payload of payloads) {
      expect(payload).not.toHaveProperty('organization_invite_code')
    }
  })

  test('keeps the WeChat invitation in a header outside the request URL', async () => {
    const requests: Array<{ url: string; config?: ApiConfig }> = []
    apiClient.get = async (url, config) => {
      requests.push({ url, config })
      return { data: { success: false, message: '' } }
    }

    await wechatLoginByCode('wechat-code', '  ORG-CODE  ')

    expect(requests).toEqual([
      {
        url: '/api/oauth/wechat',
        config: {
          params: { code: 'wechat-code' },
          headers: { 'X-Organization-Invite-Code': 'ORG-CODE' },
        },
      },
    ])
    expect(requests[0]?.config?.params).not.toHaveProperty(
      'organization_invite_code'
    )
  })

  test('keeps the Telegram invitation in a header outside signed query fields', async () => {
    const requests: Array<{ url: string; config?: ApiConfig }> = []
    apiClient.get = async (url, config) => {
      requests.push({ url, config })
      return { data: { success: false, message: '' } }
    }
    const authorization: TelegramAuthorization = {
      id: 42,
      auth_date: 1_700_000_000,
      hash: 'signed-hash',
      username: 'telegram-user',
    }

    await telegramLogin(authorization, '  ORG-CODE  ')

    expect(requests).toEqual([
      {
        url: '/api/oauth/telegram/login',
        config: {
          params: authorization,
          headers: { 'X-Organization-Invite-Code': 'ORG-CODE' },
          disableDuplicate: true,
          skipAuthRefresh: true,
          skipBusinessError: true,
          skipErrorHandler: true,
        },
      },
    ])
    expect(requests[0]?.config?.params).not.toHaveProperty(
      'organization_invite_code'
    )
  })
})
