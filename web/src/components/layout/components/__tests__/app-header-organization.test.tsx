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
import { render, screen } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import {
  getTenantOrganizationSummary,
  tenantOrganizationKeys,
} from '@/features/organizations/tenant-api'
import type { TenantOrganizationSummary } from '@/features/organizations/types'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AppHeader } from '../app-header'

vi.mock('@/features/organizations/tenant-api', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/organizations/tenant-api')>()

  return {
    ...actual,
    getTenantOrganizationSummary: vi.fn(),
  }
})

vi.mock('@/hooks/use-notifications', () => ({
  useNotifications: () => ({}),
}))

vi.mock('@/hooks/use-top-nav-links', () => ({
  useTopNavLinks: () => [],
}))

vi.mock('../header', () => ({
  Header: (props: PropsWithChildren) => <header>{props.children}</header>,
}))

vi.mock('../system-brand', () => ({
  SystemBrand: (props: {
    organization?: Pick<TenantOrganizationSummary, 'name'>
  }) => (
    <span data-testid='header-organization'>{props.organization?.name}</span>
  ),
}))

const organizationSummary: TenantOrganizationSummary = {
  organization_id: 17,
  name: 'Example Organization',
  is_default: false,
  status: 'active',
  current_user_role: 'member',
  member_status: 'active',
  allow_member_topup: false,
}

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
  vi.clearAllMocks()
})

describe('app header organization summary', () => {
  test('does not fetch or reuse a cached summary for a default organization member', () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    queryClient.setQueryData(
      tenantOrganizationKeys.summary(),
      organizationSummary
    )
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'default-member',
      role: ROLE.USER,
      organization_id: 17,
      organization_role: 'member',
      organization_status: 'active',
      organization_is_default: true,
    })

    render(
      <QueryClientProvider client={queryClient}>
        <AppHeader rightContent={<div />} />
      </QueryClientProvider>
    )

    expect(getTenantOrganizationSummary).not.toHaveBeenCalled()
    expect(screen.getByTestId('header-organization')).toBeEmptyDOMElement()
  })

  test('fetches and displays the matching summary for a non-default organization member', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    vi.mocked(getTenantOrganizationSummary).mockResolvedValue(
      organizationSummary
    )
    useAuthStore.getState().auth.setUser({
      id: 2,
      username: 'tenant-member',
      role: ROLE.USER,
      organization_id: 17,
      organization_role: 'member',
      organization_status: 'active',
      organization_is_default: false,
    })

    render(
      <QueryClientProvider client={queryClient}>
        <AppHeader rightContent={<div />} />
      </QueryClientProvider>
    )

    expect(await screen.findByText('Example Organization')).toBeVisible()
    expect(getTenantOrganizationSummary).toHaveBeenCalledOnce()
  })
})
