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
import { isRedirect } from '@tanstack/react-router'
import { afterEach, describe, expect, test, vi } from 'vitest'

import {
  LOG_TYPE_ALL_VALUE,
  ORGANIZATION_LOG_TYPE_VALUE,
} from '@/features/usage-logs/constants'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { Route } from '../$section'

vi.mock('@/features/usage-logs', () => ({ UsageLogs: () => null }))

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

function invokeBeforeLoad(section: string, type: string[]): unknown {
  const beforeLoad = Route.options.beforeLoad
  expect(beforeLoad).toBeTypeOf('function')
  return beforeLoad?.({ params: { section }, search: { type } } as never)
}

function activeTenantMember(isDefault: boolean) {
  useAuthStore.getState().auth.setUser({
    id: 7,
    username: isDefault ? 'default-member' : 'tenant-member',
    role: ROLE.USER,
    organization_id: isDefault ? 1 : 17,
    organization_role: 'member',
    organization_status: 'active',
    organization_is_default: isDefault,
  })
}

function catchRedirect(callback: () => unknown) {
  try {
    callback()
  } catch (error) {
    expect(isRedirect(error)).toBe(true)
    return error
  }
  throw new Error('Expected route redirect')
}

describe('usage-log organization category access', () => {
  test('redirects an eligible legacy organization URL to the usage-log category', () => {
    activeTenantMember(false)

    const redirect = catchRedirect(() =>
      invokeBeforeLoad('organization', [LOG_TYPE_ALL_VALUE])
    )

    if (!isRedirect(redirect)) return
    expect(redirect.options.to).toBe('/usage-logs/$section')
    expect(redirect.options.params).toEqual({ section: 'common' })
    expect(redirect.options.search).toMatchObject({
      type: [ORGANIZATION_LOG_TYPE_VALUE],
    })
  })

  test('allows an eligible member to select the organization category', () => {
    activeTenantMember(false)

    expect(() =>
      invokeBeforeLoad('common', [ORGANIZATION_LOG_TYPE_VALUE])
    ).not.toThrow()
  })

  test('clears the organization category for a default-organization member', () => {
    activeTenantMember(true)

    const redirect = catchRedirect(() =>
      invokeBeforeLoad('common', [ORGANIZATION_LOG_TYPE_VALUE])
    )

    if (!isRedirect(redirect)) return
    expect(redirect.options.search).toMatchObject({
      type: [LOG_TYPE_ALL_VALUE],
    })
  })
})
