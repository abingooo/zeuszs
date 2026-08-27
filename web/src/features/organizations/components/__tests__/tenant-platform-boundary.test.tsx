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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import { api } from '@/lib/api'

import { OrganizationWorkspace } from '../../organization-workspace'
import type { OrganizationRole, TenantOrganizationSummary } from '../../types'

type ApiGet = (
  url: string,
  config?: unknown
) => Promise<{ data: Record<string, unknown> }>

const apiClient = api as unknown as { get: ApiGet }
const originalGet = apiClient.get

afterEach(() => {
  apiClient.get = originalGet
})

function managementSummary(
  currentUserRole: Extract<OrganizationRole, 'owner' | 'admin'>
): TenantOrganizationSummary {
  return {
    organization_id: 17,
    name: 'Research Team',
    status: 'active',
    current_user_role: currentUserRole,
    member_status: 'active',
    allow_member_topup: true,
    owner_user_id: 1,
    policy_version: 1,
    fund_quota: 5000,
    member_count: 3,
    created_at: 1_700_000_000,
    updated_at: 1_700_000_000,
  }
}

describe('tenant workspace platform boundary', () => {
  test.each(['owner', 'admin'] as const)(
    'uses tenant APIs and does not expose platform controls to organization %s',
    async (organizationRole) => {
      const calls: string[] = []
      apiClient.get = async (url) => {
        calls.push(url)
        if (url === '/api/organization/self') {
          return {
            data: { success: true, data: managementSummary(organizationRole) },
          }
        }
        if (
          url === '/api/organization/members' ||
          url === '/api/organization/invites' ||
          url === '/api/organization/ledger' ||
          url === '/api/organization/audit'
        ) {
          return {
            data: {
              success: true,
              data: { items: [], total: 0, page: 1, page_size: 10 },
            },
          }
        }
        throw new Error(`Unexpected organization API request: ${url}`)
      }
      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })

      render(
        <QueryClientProvider client={queryClient}>
          <OrganizationWorkspace />
        </QueryClientProvider>
      )

      for (const tabName of ['Members', 'Invites', 'Ledger', 'Audit']) {
        fireEvent.click(await screen.findByRole('tab', { name: tabName }))
      }
      await waitFor(() => {
        expect(calls).toEqual(
          expect.arrayContaining([
            '/api/organization/self',
            '/api/organization/members',
            '/api/organization/invites',
            '/api/organization/ledger',
            '/api/organization/audit',
          ])
        )
      })

      expect(calls.every((url) => !url.includes('/admin'))).toBe(true)
      expect(
        screen.queryByRole('button', { name: 'Create organization' })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Create account' })
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: 'Credit pool' })
      ).not.toBeInTheDocument()
      expect(screen.queryByText('Make Owner')).not.toBeInTheDocument()
    }
  )
})
