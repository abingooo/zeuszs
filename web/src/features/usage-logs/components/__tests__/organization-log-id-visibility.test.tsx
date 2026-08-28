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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { OrganizationUsageLog } from '../../types'
import { useOrganizationLogsColumns } from '../columns/organization-logs-columns'
import { UsageLogsMobileList } from '../usage-logs-mobile-card'

const organizationLog: OrganizationUsageLog = {
  id: 1,
  organization_id: 17,
  organization_name: 'Alpha Lab',
  actor_user_id: 42,
  actor_username: 'alice',
  initiator_user_id: 43,
  initiator_username: 'carol',
  action: 'organization.quota.allocate',
  target_type: 'user',
  target_id: '88',
  target_name: 'bob',
  request_id: 'request-1',
  metadata: { user_quota_delta: 1_000 },
  created_at: 1_700_000_000,
}

function OrganizationLogLayouts() {
  const columns = useOrganizationLogsColumns()
  const table = useReactTable({
    data: [organizationLog],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  const row = table.getRowModel().rows[0]

  return (
    <>
      <div data-testid='desktop-organization-log'>
        {row.getVisibleCells().map((cell) => (
          <div key={cell.id}>
            {flexRender(cell.column.columnDef.cell, cell.getContext())}
          </div>
        ))}
      </div>
      <div data-testid='mobile-organization-log'>
        <UsageLogsMobileList table={table} logCategory='organization' />
      </div>
    </>
  )
}

function setIDVisibility(enabled: boolean) {
  useAuthStore.getState().auth.setUser({
    id: 7,
    username: 'tenant-admin',
    role: ROLE.USER,
    organization_id: organizationLog.organization_id,
    organization_role: 'admin',
    organization_status: 'active',
    organization_is_default: false,
    permissions: { id_visible: enabled },
  })
}

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

describe('organization log ID visibility', () => {
  test('hides organization and user IDs in desktop and mobile layouts', () => {
    setIDVisibility(false)

    render(<OrganizationLogLayouts />)

    for (const testID of [
      'desktop-organization-log',
      'mobile-organization-log',
    ]) {
      const layout = within(screen.getByTestId(testID))
      expect(layout.queryByText('#17')).not.toBeInTheDocument()
      expect(layout.queryByText('alice (#42)')).not.toBeInTheDocument()
      expect(layout.queryByText(/carol/)).not.toBeInTheDocument()
      expect(layout.queryByText(/bob \(#88\)/)).not.toBeInTheDocument()
      expect(layout.getByText('alice')).toBeVisible()
      expect(layout.getByText(/^bob ·/)).toBeVisible()
    }
  })

  test('shows organization and user IDs in desktop and mobile layouts', () => {
    setIDVisibility(true)

    render(<OrganizationLogLayouts />)

    for (const testID of [
      'desktop-organization-log',
      'mobile-organization-log',
    ]) {
      const layout = within(screen.getByTestId(testID))
      expect(layout.getByText('#17')).toBeVisible()
      expect(layout.getByText('alice (#42)')).toBeVisible()
      expect(layout.queryByText(/carol/)).not.toBeInTheDocument()
      expect(layout.getByText(/^bob \(#88\) ·/)).toBeVisible()
    }
  })
})
