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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { OrganizationWorkspaceView } from '../../organization-workspace'
import { tenantOrganizationKeys } from '../../tenant-api'
import type { OrganizationMember, TenantOrganizationSummary } from '../../types'
import { TenantOrganizationMembersPanel } from '../tenant-organization-members-panel'

const memberSummary: TenantOrganizationSummary = {
  organization_id: 7,
  name: 'Research Team',
  status: 'active',
  current_user_role: 'member',
  member_status: 'active',
  allow_member_topup: true,
}

const managementSummary: TenantOrganizationSummary = {
  ...memberSummary,
  owner_user_id: 1,
  current_user_role: 'admin',
  policy_version: 1,
  fund_quota: 5000,
  member_count: 3,
  created_at: 1_700_000_000,
  updated_at: 1_700_000_000,
}

describe('tenant organization authority', () => {
  test('shows an ordinary member only the organization summary', () => {
    render(<OrganizationWorkspaceView summary={memberSummary} />)

    expect(screen.getByText('Research Team')).toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(screen.queryByText('Members')).not.toBeInTheDocument()
    expect(
      screen.queryByText('Organization pool balance')
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(
        'Your organization administrators manage member access, quotas, and registration policy.'
      )
    ).toBeInTheDocument()
  })

  test('ordinary member row actions do not expose role or ownership changes', async () => {
    const member: OrganizationMember = {
      user_id: 8,
      username: 'ordinary-member',
      display_name: 'Ordinary Member',
      email: 'member@example.com',
      platform_role: 1,
      organization_id: 7,
      organization_role: 'member',
      organization_status: 'active',
      quota: 1000,
      used_quota: 0,
      request_count: 0,
      recoverable_quota: 1000,
      consumed_quota: 0,
      created_at: 1_700_000_000,
    }
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      },
    })
    queryClient.setQueryData(
      tenantOrganizationKeys.memberList({
        organizationId: 7,
        page: 1,
        pageSize: 10,
        keyword: '',
      }),
      { items: [member], total: 1, page: 1, page_size: 10 }
    )

    render(
      <QueryClientProvider client={queryClient}>
        <TenantOrganizationMembersPanel summary={managementSummary} />
      </QueryClientProvider>
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Manage member' })
    )

    const menuItems = await screen.findAllByRole('menuitem')
    expect(menuItems.map((item) => item.textContent)).toEqual([
      'Allocate quota',
      'Recover quota',
      'Set consumption limit',
      'Disable API keys',
      'Disable member',
    ])
    expect(screen.queryByText('Make Owner')).not.toBeInTheDocument()
    expect(screen.queryByText('Organization admin')).not.toBeInTheDocument()
  })
})
