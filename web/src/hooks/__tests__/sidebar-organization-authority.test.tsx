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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { type AuthUser, useAuthStore } from '@/stores/auth-store'

import { useSidebarView } from '../use-sidebar-view'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useLocation: (options: {
      select: (location: { pathname: string }) => unknown
    }) => options.select({ pathname: '/organization' }),
  }
})

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

function tenantManager(organizationRole: 'owner' | 'admin'): AuthUser {
  return {
    id: organizationRole === 'owner' ? 1 : 2,
    username: `tenant-${organizationRole}`,
    role: ROLE.USER,
    organization_id: 17,
    organization_role: organizationRole,
    organization_status: 'active',
  }
}

function queryWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
    },
  })
  queryClient.setQueryData(['status'], { SidebarModulesAdmin: '' })

  return function Wrapper(props: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        {props.children}
      </QueryClientProvider>
    )
  }
}

describe('organization sidebar authority', () => {
  test.each(['owner', 'admin'] as const)(
    'hides platform organization navigation from tenant %s with User platform role',
    (organizationRole) => {
      useAuthStore.getState().auth.setUser(tenantManager(organizationRole))

      const { result } = renderHook(() => useSidebarView(), {
        wrapper: queryWrapper(),
      })

      expect(
        result.current.navGroups.some((group) => group.id === 'admin')
      ).toBe(false)
      expect(
        result.current.navGroups
          .flatMap((group) => group.items)
          .some((item) => 'url' in item && item.url === '/organizations')
      ).toBe(false)
      expect(
        result.current.navGroups
          .flatMap((group) => group.items)
          .some((item) => 'url' in item && item.url === '/organization')
      ).toBe(true)
    }
  )

  test('hides organization navigation from a default-organization member', () => {
    useAuthStore.getState().auth.setUser({
      id: 3,
      username: 'default-member',
      role: ROLE.USER,
      organization_id: 1,
      organization_role: 'member',
      organization_status: 'active',
      organization_is_default: true,
    })

    const { result } = renderHook(() => useSidebarView(), {
      wrapper: queryWrapper(),
    })

    expect(
      result.current.navGroups
        .flatMap((group) => group.items)
        .some((item) => 'url' in item && item.url === '/organization')
    ).toBe(false)
  })
})
