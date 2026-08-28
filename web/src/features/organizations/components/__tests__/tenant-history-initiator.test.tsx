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
import { useQuery } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { TenantOrganizationAuditPanel } from '../tenant-organization-history-panels'

vi.mock('@tanstack/react-query', () => ({
  keepPreviousData: Symbol('keepPreviousData'),
  useQuery: vi.fn(),
}))

vi.mock('../../tenant-api', () => ({
  listTenantOrganizationAudit: vi.fn(),
  listTenantOrganizationLedger: vi.fn(),
  tenantOrganizationKeys: {
    auditList: () => ['organization', 'audit'],
    ledgerList: () => ['organization', 'ledger'],
  },
}))

vi.mock('../organization-query-error', () => ({
  OrganizationQueryError: () => null,
}))

vi.mock('../pagination-controls', () => ({
  PaginationControls: () => null,
}))

beforeEach(() => {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'platform-admin',
    role: ROLE.ADMIN,
  })
  vi.mocked(useQuery).mockReturnValue({
    data: {
      items: [
        {
          id: 1,
          actor_user_id: 0,
          initiator_user_id: 42,
          action: 'organization.fund.credit',
          target_type: 'organization',
          target_id: '7',
          request_id: 'request-1',
          metadata: {},
          created_at: 1_700_000_000,
        },
      ],
      total: 1,
    },
    isLoading: false,
    isError: false,
    isFetching: false,
    refetch: vi.fn(),
  } as never)
})

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

describe('organization audit initiator', () => {
  test('shows the payer separately from the system accounting actor', () => {
    render(<TenantOrganizationAuditPanel />)

    expect(screen.getByText('Actor user ID')).toBeVisible()
    expect(screen.getByText('Initiator user ID')).toBeVisible()
    expect(screen.getByText('0')).toBeVisible()
    expect(screen.getByText('42')).toBeVisible()
  })

  test('hides actor, initiator, and target IDs for ordinary members', () => {
    useAuthStore.getState().auth.setUser({
      id: 7,
      username: 'member',
      role: ROLE.USER,
      organization_id: 7,
      organization_role: 'member',
      organization_status: 'active',
      organization_is_default: false,
    })

    render(<TenantOrganizationAuditPanel />)

    expect(screen.queryByText('Actor user ID')).not.toBeInTheDocument()
    expect(screen.queryByText('Initiator user ID')).not.toBeInTheDocument()
    expect(screen.queryByText('0')).not.toBeInTheDocument()
    expect(screen.queryByText('42')).not.toBeInTheDocument()
    expect(screen.getByText('organization')).toBeVisible()
    expect(screen.queryByText('organization:7')).not.toBeInTheDocument()
  })
})
