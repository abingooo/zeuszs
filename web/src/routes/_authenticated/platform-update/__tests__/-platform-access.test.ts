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

import { Route } from '../index'

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

function invokeBeforeLoad(): void {
  const beforeLoad = Route.options.beforeLoad
  expect(beforeLoad).toBeTypeOf('function')
  beforeLoad?.({} as never)
}

describe('/platform-update access', () => {
  test.each([ROLE.GUEST, ROLE.USER])('redirects platform role %s', (role) => {
    if (role !== ROLE.GUEST) {
      useAuthStore.getState().auth.setUser({
        id: role,
        username: `role-${role}`,
        role,
      })
    }

    let caught: unknown
    try {
      invokeBeforeLoad()
    } catch (error) {
      caught = error
    }

    expect(isRedirect(caught)).toBe(true)
    if (isRedirect(caught)) expect(caught.options.to).toBe('/403')
  })

  test.each([ROLE.ADMIN, ROLE.SUPER_ADMIN])(
    'allows platform role %s',
    (role) => {
      useAuthStore.getState().auth.setUser({
        id: role,
        username: `role-${role}`,
        role,
      })

      expect(() => invokeBeforeLoad()).not.toThrow()
    }
  )
})
