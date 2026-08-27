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
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import type { SystemStatus } from '../../types'
import { useOAuthLogin } from '../use-oauth-login'

const mocks = vi.hoisted(() => ({
  clearAuthentication: vi.fn(),
  createOAuthFlow: vi.fn(async () => 'oauth-state'),
  handleLoginSuccess: vi.fn(),
  logout: vi.fn(async () => ({ success: true, message: '' })),
  telegramLogin: vi.fn(async () => ({ success: false, message: '' })),
  toastError: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  clearAuthentication: mocks.clearAuthentication,
  isAuthBundle: () => false,
}))

vi.mock('../../api', () => ({
  createOAuthFlow: mocks.createOAuthFlow,
  logout: mocks.logout,
  telegramLogin: mocks.telegramLogin,
}))

vi.mock('../use-auth-redirect', () => ({
  useAuthRedirect: () => ({
    handleLoginSuccess: mocks.handleLoginSuccess,
  }),
}))

vi.mock('sonner', () => ({
  toast: {
    error: mocks.toastError,
    success: vi.fn(),
  },
}))

const status = {
  discord_oauth: true,
  discord_client_id: 'discord-client',
  telegram_oauth: true,
  telegram_bot_name: 'zeuszs_bot',
} satisfies SystemStatus

beforeEach(() => {
  vi.spyOn(window, 'open').mockImplementation(() => null)
})

describe('OAuth organization invitation propagation', () => {
  test('passes the invitation to a standard OAuth flow', async () => {
    const { result } = renderHook(() =>
      useOAuthLogin(status, undefined, 'ORG-CODE')
    )

    await act(async () => {
      await result.current.handleDiscordLogin()
    })

    expect(mocks.createOAuthFlow).toHaveBeenCalledWith(
      'discord',
      'login',
      'ORG-CODE'
    )
  })

  test('passes the invitation to a custom OAuth flow', async () => {
    const { result } = renderHook(() =>
      useOAuthLogin(status, undefined, 'ORG-CODE')
    )

    await act(async () => {
      await result.current.handleCustomOAuthLogin({
        id: 7,
        name: 'Corporate SSO',
        slug: 'corporate-sso',
        icon: '',
        client_id: 'custom-client',
        authorization_endpoint: 'https://sso.example.com/authorize',
        scopes: 'openid profile',
      })
    })

    expect(mocks.createOAuthFlow).toHaveBeenCalledWith(
      'corporate-sso',
      'login',
      'ORG-CODE'
    )
  })

  test('passes the invitation separately from Telegram authorization data', async () => {
    const authorization = {
      id: 42,
      auth_date: 1_700_000_000,
      hash: 'signed-hash',
      username: 'telegram-user',
    }
    const { result } = renderHook(() =>
      useOAuthLogin(status, undefined, 'ORG-CODE')
    )

    await act(async () => {
      await result.current.handleTelegramAuthorization(authorization)
    })

    expect(mocks.telegramLogin).toHaveBeenCalledWith(authorization, 'ORG-CODE')
  })
})
