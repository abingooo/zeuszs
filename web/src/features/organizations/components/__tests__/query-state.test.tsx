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
import { formatQuota } from '@/lib/format'

import { OrganizationsList } from '../organizations-list'

type ApiGet = (
  url: string,
  config?: unknown
) => Promise<{ data: Record<string, unknown> }>

const apiClient = api as unknown as { get: ApiGet }
const originalGet = apiClient.get

afterEach(() => {
  apiClient.get = originalGet
})

describe('organization query states', () => {
  test('shows a load error instead of an empty state and recovers on retry', async () => {
    let requestCount = 0
    apiClient.get = async () => {
      requestCount += 1
      if (requestCount === 1) throw new Error('offline')

      return {
        data: {
          success: true,
          data: {
            items: [
              {
                id: 7,
                name: 'Research Team',
                status: 'active',
                owner_user_id: 1,
                owner_username: 'owner',
                allow_member_topup: false,
                policy_version: 1,
                member_count: 2,
                fund_quota: 2500000,
                created_at: 1_700_000_000,
                updated_at: 1_700_000_000,
              },
            ],
            total: 1,
            page: 1,
            page_size: 20,
          },
        },
      }
    }
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <OrganizationsList
          onCreate={() => undefined}
          onManage={() => undefined}
        />
      </QueryClientProvider>
    )

    expect(await screen.findByText('Failed to load')).toBeInTheDocument()
    expect(screen.queryByText('No organizations found')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

    expect(await screen.findAllByText('Research Team')).toHaveLength(2)
    expect(screen.getAllByText(formatQuota(2_500_000))).toHaveLength(2)
    await waitFor(() => expect(requestCount).toBe(2))
    expect(screen.queryByText('Failed to load')).not.toBeInTheDocument()
  })
})
