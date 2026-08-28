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
import { beforeEach, expect, test, vi } from 'vitest'

import { fetchLogsByCategory } from '../../lib/utils'
import { UsageLogsTable } from '../usage-logs-table'

vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({
    useSearch: () => ({ type: ['organization'] }),
    useNavigate: () => vi.fn(),
  }),
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: () => <div>data table</div>,
  DataTableRow: () => null,
  useDataTable: () => ({ table: {} }),
}))

vi.mock('@/hooks', () => ({
  useMediaQuery: () => false,
}))

vi.mock('@/hooks/use-table-url-state', () => ({
  useTableUrlState: () => ({
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    ensurePageInRange: vi.fn(),
  }),
}))

vi.mock('../../lib/columns', () => ({
  useColumnsByCategory: () => [],
}))

vi.mock('../../lib/utils', () => ({
  fetchLogsByCategory: vi.fn(),
}))

vi.mock('../common-logs-filter-bar', () => ({
  CommonLogsFilterBar: () => <div>usage log filters</div>,
}))

vi.mock('../task-logs-filter-bar', () => ({
  TaskLogsFilterBar: () => null,
}))

vi.mock('../usage-logs-mobile-card', () => ({
  UsageLogsMobileList: () => null,
}))

vi.mock('../usage-logs-provider', () => ({
  useLogsViewScope: () => ({ canManageScope: false, isAdminView: false }),
}))

beforeEach(() => {
  vi.mocked(fetchLogsByCategory).mockResolvedValue({
    success: false,
    message: 'organization logs unavailable',
  })
})

test('shows a retryable error instead of the empty organization-log state', async () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  render(
    <QueryClientProvider client={queryClient}>
      <UsageLogsTable logCategory='common' />
    </QueryClientProvider>
  )

  expect(await screen.findByText('Failed to load logs')).toBeVisible()
  expect(screen.getByText('Please try again later.')).toBeVisible()
  expect(screen.getByText('usage log filters')).toBeVisible()
  expect(screen.queryByText('data table')).not.toBeInTheDocument()

  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

  await waitFor(() => {
    expect(fetchLogsByCategory).toHaveBeenCalledTimes(2)
  })
  expect(fetchLogsByCategory).toHaveBeenCalledWith(
    expect.objectContaining({ logCategory: 'organization' })
  )
  queryClient.clear()
})
