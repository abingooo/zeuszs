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
import { afterEach, describe, expect, test } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { Route } from './index'

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

function invokeBeforeLoad(): void {
  const beforeLoad = Route.options.beforeLoad
  expect(beforeLoad).toBeTypeOf('function')
  beforeLoad?.({} as never)
}

describe('/organization default member access', () => {
  test('redirects an ordinary member of the default organization', () => {
    useAuthStore.getState().auth.setUser({
      id: 7,
      username: 'default-member',
      role: ROLE.USER,
      organization_id: 1,
      organization_role: 'member',
      organization_status: 'active',
      organization_is_default: true,
    })

    let caught: unknown
    try {
      invokeBeforeLoad()
    } catch (error) {
      caught = error
    }

    expect(isRedirect(caught)).toBe(true)
    if (!isRedirect(caught)) return
    expect(caught.options.to).toBe('/dashboard/$section')
    expect(caught.options.params).toEqual({ section: 'models' })
  })

  test('allows a member of a tenant organization', () => {
    useAuthStore.getState().auth.setUser({
      id: 8,
      username: 'tenant-member',
      role: ROLE.USER,
      organization_id: 17,
      organization_role: 'member',
      organization_status: 'active',
      organization_is_default: false,
    })

    expect(() => invokeBeforeLoad()).not.toThrow()
  })
})
