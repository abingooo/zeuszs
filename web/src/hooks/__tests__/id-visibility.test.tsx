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
import { afterEach, describe, expect, test } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { canViewInternalIds, useIdVisibility } from '../use-id-visibility'

function user(role: number, idVisible?: boolean): AuthUser {
  return {
    id: 1,
    username: 'tester',
    role,
    permissions:
      idVisible === undefined ? undefined : { id_visible: idVisible },
  }
}

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

describe('internal ID visibility', () => {
  test.each([
    ['guest without permission', null, false],
    ['user without permission', user(ROLE.USER), false],
    ['user with disabled permission', user(ROLE.USER, false), false],
    ['user with enabled permission', user(ROLE.USER, true), true],
    ['platform admin with disabled permission', user(ROLE.ADMIN, false), true],
    [
      'platform owner with disabled permission',
      user(ROLE.SUPER_ADMIN, false),
      true,
    ],
  ] as const)('%s', (_name, currentUser, expected) => {
    expect(canViewInternalIds(currentUser)).toBe(expected)
  })

  test('hook follows changes to the authenticated user permission', () => {
    useAuthStore.getState().auth.setUser(user(ROLE.USER, false))

    const { result } = renderHook(() => useIdVisibility())

    expect(result.current).toBe(false)

    act(() => {
      useAuthStore.getState().auth.setUser(user(ROLE.USER, true))
    })

    expect(result.current).toBe(true)
  })
})
