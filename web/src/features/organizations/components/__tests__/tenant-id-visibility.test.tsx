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
import { afterEach, describe, expect, test } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { tenantOrganizationKeys } from '../../tenant-api'
import type { OrganizationMember, TenantOrganizationSummary } from '../../types'
import { TenantOrganizationMembersPanel } from '../tenant-organization-members-panel'
import { TenantOrganizationOverview } from '../tenant-organization-overview'

const summary: TenantOrganizationSummary = {
  organization_id: 17,
  name: 'Alpha Lab',
  status: 'active',
  current_user_role: 'admin',
  member_status: 'active',
  allow_member_topup: true,
  owner_user_id: 1,
  policy_version: 1,
  fund_quota: 5_000,
  member_count: 1,
  created_at: 1_700_000_000,
  updated_at: 1_700_000_000,
}

const member: OrganizationMember = {
  user_id: 88,
  username: 'member-user',
  display_name: 'Member User',
  email: 'member@example.com',
  platform_role: ROLE.USER,
  organization_id: summary.organization_id,
  organization_role: 'member',
  organization_status: 'active',
  quota: 1_000,
  used_quota: 100,
  request_count: 4,
  recoverable_quota: 900,
  consumed_quota: 100,
  created_at: 1_700_000_000,
}

function setIDVisibility(enabled: boolean) {
  useAuthStore.getState().auth.setUser({
    id: 7,
    username: 'tenant-admin',
    role: ROLE.USER,
    organization_id: summary.organization_id,
    organization_role: 'admin',
    organization_status: 'active',
    organization_is_default: false,
    permissions: { id_visible: enabled },
  })
}

function renderMembersPanel() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
    },
  })
  queryClient.setQueryData(
    tenantOrganizationKeys.memberList({
      organizationId: summary.organization_id,
      page: 1,
      pageSize: 10,
      keyword: '',
    }),
    { items: [member], total: 1, page: 1, page_size: 10 }
  )

  return {
    queryClient,
    ...render(
      <QueryClientProvider client={queryClient}>
        <TenantOrganizationMembersPanel summary={summary} />
      </QueryClientProvider>
    ),
  }
}

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

describe('tenant organization ID visibility', () => {
  test('hides the organization ID in the overview when visibility is disabled', () => {
    setIDVisibility(false)

    render(<TenantOrganizationOverview summary={summary} />)

    expect(screen.queryByText('Organization ID: 17')).not.toBeInTheDocument()
  })

  test('shows the organization ID in the overview when visibility is enabled', () => {
    setIDVisibility(true)

    render(<TenantOrganizationOverview summary={summary} />)

    expect(screen.getByText('Organization ID: 17')).toBeVisible()
  })

  test('hides member user IDs when visibility is disabled', async () => {
    setIDVisibility(false)
    const { queryClient } = renderMembersPanel()

    expect(await screen.findByText('@member-user')).toBeVisible()
    expect(screen.queryByText('ID: 88')).not.toBeInTheDocument()

    queryClient.clear()
  })

  test('shows member user IDs when visibility is enabled', async () => {
    setIDVisibility(true)
    const { queryClient } = renderMembersPanel()

    expect(await screen.findByText(/ID: 88/)).toBeVisible()

    queryClient.clear()
  })
})
