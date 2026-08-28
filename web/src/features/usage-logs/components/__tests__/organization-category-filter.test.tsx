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
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { CommonLogsFilterBar } from '../common-logs-filter-bar'

let routeSearch: Record<string, unknown> = { type: ['0'] }
const { navigateMock } = vi.hoisted(() => ({ navigateMock: vi.fn() }))

vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({ useSearch: () => routeSearch }),
  useNavigate: () => navigateMock,
}))

vi.mock('../compact-date-time-range-picker', () => ({
  CompactDateTimeRangePicker: () => <div>date range</div>,
}))

vi.mock('../common-logs-stats', () => ({
  CommonLogsStats: () => <div>usage stats</div>,
}))

vi.mock('../logs-filter-toolbar', () => ({
  LogsFilterField: (props: { children: ReactNode }) => props.children,
  LogsFilterInput: (props: Record<string, unknown>) => <input {...props} />,
  LogsFilterToolbar: (props: {
    primaryFilters: ReactNode
    advancedFilters?: ReactNode
    stats?: ReactNode
    onSearch: () => void
  }) => (
    <div>
      {props.primaryFilters}
      {props.advancedFilters && <div>advanced filters</div>}
      {props.stats}
      <button type='button' onClick={props.onSearch}>
        Search
      </button>
    </div>
  ),
}))

vi.mock('../usage-logs-provider', () => ({
  useLogsViewScope: () => ({ isAdminView: false }),
  useUsageLogsContext: () => ({
    sensitiveVisible: true,
    setSensitiveVisible: vi.fn(),
  }),
}))

function renderFilter() {
  const queryClient = new QueryClient()
  return render(
    <QueryClientProvider client={queryClient}>
      <CommonLogsFilterBar table={{} as never} />
    </QueryClientProvider>
  )
}

function setTenant(isDefault: boolean) {
  useAuthStore.getState().auth.setUser({
    id: 7,
    username: isDefault ? 'default-member' : 'tenant-member',
    role: ROLE.USER,
    organization_id: isDefault ? 1 : 17,
    organization_role: 'member',
    organization_status: 'active',
    organization_is_default: isDefault,
  })
}

afterEach(() => {
  routeSearch = { type: ['0'] }
  navigateMock.mockReset()
  useAuthStore.getState().auth.reset('idle')
})

describe('organization category in usage-log filters', () => {
  test('selects Organization on the common usage-log route', async () => {
    const user = userEvent.setup()
    setTenant(false)
    renderFilter()

    await user.click(screen.getByRole('combobox'))
    await user.click(
      await screen.findByRole('option', { name: 'Organization' })
    )
    await user.click(screen.getByRole('button', { name: 'Search' }))

    expect(navigateMock).toHaveBeenCalledWith(
      expect.objectContaining({
        params: { section: 'common' },
        search: expect.objectContaining({ type: ['organization'], page: 1 }),
      })
    )
  })

  test('does not offer Organization to a default-organization member', () => {
    setTenant(true)
    renderFilter()

    fireEvent.click(screen.getByRole('combobox'))

    expect(
      screen.queryByRole('option', { name: 'Organization' })
    ).not.toBeInTheDocument()
  })

  test('shows only date and category controls in organization mode', () => {
    routeSearch = { type: ['organization'] }
    setTenant(false)
    renderFilter()

    expect(screen.getByText('date range')).toBeVisible()
    expect(screen.getByRole('combobox')).toHaveTextContent('Organization')
    expect(screen.queryByPlaceholderText('Model Name')).not.toBeInTheDocument()
    expect(screen.queryByPlaceholderText('Group')).not.toBeInTheDocument()
    expect(screen.queryByText('advanced filters')).not.toBeInTheDocument()
    expect(screen.queryByText('usage stats')).not.toBeInTheDocument()
  })
})
