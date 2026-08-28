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
import { useAuthStore } from '@/stores/auth-store'

import { useSidebarView } from '../use-sidebar-view'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useLocation: (options: {
      select: (location: { pathname: string }) => unknown
    }) => options.select({ pathname: '/dashboard/overview' }),
  }
})

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

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

function hasPlatformUpdateItem(
  groups: ReturnType<typeof useSidebarView>['navGroups']
): boolean {
  return groups
    .flatMap((group) => group.items)
    .some((item) => 'url' in item && item.url === '/platform-update')
}

describe('platform update sidebar authority', () => {
  test('hides the platform update entry from a regular user', () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'user',
      role: ROLE.USER,
    })

    const { result } = renderHook(() => useSidebarView(), {
      wrapper: queryWrapper(),
    })

    expect(hasPlatformUpdateItem(result.current.navGroups)).toBe(false)
  })

  test.each([ROLE.ADMIN, ROLE.SUPER_ADMIN])(
    'shows the platform update entry to platform role %s',
    (role) => {
      useAuthStore.getState().auth.setUser({
        id: role,
        username: `role-${role}`,
        role,
      })

      const { result } = renderHook(() => useSidebarView(), {
        wrapper: queryWrapper(),
      })

      expect(hasPlatformUpdateItem(result.current.navGroups)).toBe(true)
    }
  )
})
