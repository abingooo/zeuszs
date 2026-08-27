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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { UserAuthForm } from '../../../sign-in/components/user-auth-form'
import { SignUpForm } from '../sign-up-form'

type OAuthProvidersProps = {
  organizationInviteCode?: string
  onWeChatLogin?: () => void
}

const mocks = vi.hoisted(() => ({
  handleLoginSuccess: vi.fn(),
  login: vi.fn(async () => ({ success: false, message: '' })),
  redirectToLogin: vi.fn(),
  register: vi.fn(async () => ({ success: false, message: '' })),
  sendCode: vi.fn(async () => false),
  setTurnstileToken: vi.fn(),
  toastError: vi.fn(),
  wechatLoginByCode: vi.fn(async () => ({ success: false, message: '' })),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: ReactNode; to?: string }) => (
    <a href={props.to}>{props.children}</a>
  ),
}))

vi.mock('@/components/dialog', () => ({
  Dialog: (props: {
    children?: ReactNode
    footer?: ReactNode
    open: boolean
  }) =>
    props.open ? (
      <div role='dialog'>
        {props.children}
        {props.footer}
      </div>
    ) : null,
}))

vi.mock('@/features/auth/api', () => ({
  login: mocks.login,
  register: mocks.register,
  wechatLoginByCode: mocks.wechatLoginByCode,
}))

vi.mock('@/features/auth/components/oauth-providers', () => ({
  OAuthProviders: (props: OAuthProvidersProps) => (
    <div>
      <output
        data-testid='oauth-organization-invite'
        data-invite={props.organizationInviteCode}
      >
        {props.organizationInviteCode}
      </output>
      {props.onWeChatLogin ? (
        <button type='button' onClick={props.onWeChatLogin}>
          Mock WeChat
        </button>
      ) : null}
    </div>
  ),
}))

vi.mock('@/features/auth/hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({
    handleLoginSuccess: mocks.handleLoginSuccess,
    redirectTo2FA: vi.fn(),
    redirectToLogin: mocks.redirectToLogin,
  }),
}))

vi.mock('@/features/auth/hooks/use-email-verification', () => ({
  useEmailVerification: () => ({
    isSending: false,
    secondsLeft: 0,
    isActive: false,
    sendCode: mocks.sendCode,
  }),
}))

vi.mock('@/features/auth/hooks/use-turnstile', () => ({
  useTurnstile: () => ({
    isTurnstileEnabled: false,
    turnstileSiteKey: '',
    turnstileToken: '',
    setTurnstileToken: mocks.setTurnstileToken,
    validateTurnstile: () => true,
  }),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      github_oauth: true,
      github_client_id: 'github-client',
      wechat_login: true,
      oauth_register_enabled: true,
      password_login_enabled: true,
    },
  }),
}))

vi.mock('@/lib/passkey', () => ({
  buildAssertionResult: vi.fn(),
  isPasskeySupported: vi.fn(async () => false),
  prepareCredentialRequestOptions: vi.fn(),
}))

vi.mock('@/lib/server-error-message', () => ({
  getServerErrorMessageKey: () => null,
}))

vi.mock('sonner', () => ({
  toast: {
    error: mocks.toastError,
    info: vi.fn(),
    success: vi.fn(),
  },
}))

beforeEach(() => {
  window.history.replaceState(null, '', '/')
})

describe('organization invitation at authentication form boundaries', () => {
  test('registration passes the current invitation to OAuth and WeChat', async () => {
    const user = userEvent.setup()
    render(<SignUpForm />)

    await user.type(
      screen.getByLabelText('Organization invite code (optional)'),
      'ORG-CODE'
    )
    expect(screen.getByTestId('oauth-organization-invite')).toHaveAttribute(
      'data-invite',
      'ORG-CODE'
    )

    await user.click(screen.getByRole('button', { name: 'Mock WeChat' }))
    await user.type(screen.getByLabelText('Verification code'), 'wechat-code')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))

    expect(mocks.wechatLoginByCode).toHaveBeenCalledWith(
      'wechat-code',
      'ORG-CODE'
    )
  })

  test('ordinary login has no invitation field or OAuth and WeChat invitation', async () => {
    const user = userEvent.setup()
    render(<UserAuthForm />)

    expect(
      screen.queryByLabelText('Organization invite code (optional)')
    ).not.toBeInTheDocument()
    expect(screen.getByTestId('oauth-organization-invite')).not.toHaveAttribute(
      'data-invite'
    )

    await user.click(screen.getByRole('button', { name: 'Mock WeChat' }))
    await user.type(screen.getByLabelText('Verification code'), 'login-code')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))

    expect(mocks.wechatLoginByCode).toHaveBeenCalledWith('login-code')
  })
})
