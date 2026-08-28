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
import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useSidebarData } from '../use-sidebar-data'

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

describe('sidebar data', () => {
  test('does not expose playground or chat navigation', () => {
    const { result } = renderHook(() => useSidebarData())

    expect(result.current.navGroups.some((group) => group.id === 'chat')).toBe(
      false
    )
    expect(
      result.current.navGroups
        .flatMap((group) => group.items)
        .some((item) => {
          if ('type' in item && item.type === 'chat-presets') return true
          return 'url' in item && item.url === '/playground'
        })
    ).toBe(false)
  })

  test('exposes tenant and platform organization navigation separately', () => {
    const { result } = renderHook(() => useSidebarData())
    const personal = result.current.navGroups.find(
      (group) => group.id === 'personal'
    )
    const admin = result.current.navGroups.find((group) => group.id === 'admin')

    expect(personal?.items).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ url: '/organization' }),
      ])
    )
    expect(admin?.items).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ url: '/organizations' }),
      ])
    )
  })

  test.each(['owner', 'admin'] as const)(
    'labels the tenant workspace as Manage organization for an %s',
    (organizationRole) => {
      useAuthStore.getState().auth.setUser({
        id: 1,
        username: 'tenant-manager',
        role: ROLE.USER,
        organization_id: 17,
        organization_role: organizationRole,
        organization_status: 'active',
      })

      const { result } = renderHook(() => useSidebarData())
      const item = result.current.navGroups
        .flatMap((group) => group.items)
        .find(
          (candidate) => 'url' in candidate && candidate.url === '/organization'
        )

      expect(item?.title).toBe('Manage organization')
    }
  )

  test.each([
    {
      username: 'member',
      role: ROLE.USER,
      organization_id: 17,
      organization_role: 'member' as const,
      organization_status: 'active' as const,
      organization_is_default: false,
    },
    { username: 'platform-admin', role: ROLE.ADMIN },
    { username: 'platform-root', role: ROLE.SUPER_ADMIN },
  ])('does not expose a separate organization-log navigation item', (user) => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      ...user,
    })

    const { result } = renderHook(() => useSidebarData())
    const items = result.current.navGroups.flatMap((group) => group.items)

    expect(
      items.some(
        (candidate) =>
          'url' in candidate && candidate.url === '/usage-logs/organization'
      )
    ).toBe(false)
    expect(
      items.some(
        (candidate) =>
          'url' in candidate && candidate.url === '/usage-logs/common'
      )
    ).toBe(true)
  })
})
