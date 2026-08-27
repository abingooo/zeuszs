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
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import type { TopupRecord } from '../../types'
import { BillingHistoryDialog } from '../dialogs/billing-history-dialog'

const mocks = vi.hoisted(() => ({
  records: [] as TopupRecord[],
}))

vi.mock('@/components/dialog', () => ({
  Dialog: (props: { open: boolean; children: ReactNode }) =>
    props.open ? <div role='dialog'>{props.children}</div> : null,
}))

vi.mock('@/components/status-badge', () => ({
  StatusBadge: (props: { label: string }) => <span>{props.label}</span>,
}))

vi.mock('../../hooks/use-billing-history', () => ({
  useBillingHistory: () => ({
    records: mocks.records,
    total: mocks.records.length,
    page: 1,
    pageSize: 10,
    keyword: '',
    loading: false,
    completing: false,
    isAdmin: true,
    handlePageChange: vi.fn(),
    handlePageSizeChange: vi.fn(),
    handleSearch: vi.fn(),
    handleCompleteOrder: vi.fn(),
  }),
}))

function organizationOrder(id: number, organizationName?: string): TopupRecord {
  return {
    id,
    user_id: id,
    amount: 10,
    money: 1,
    trade_no: `order-${id}`,
    payment_method: 'stripe',
    topup_target: 'organization',
    organization_id: id,
    organization_name: organizationName,
    create_time: 1_700_000_000,
    status: 'success',
  }
}

beforeEach(() => {
  mocks.records = []
})

describe('billing history organization labels', () => {
  test('uses each order organization name in the platform-wide history', () => {
    mocks.records = [
      organizationOrder(1, 'Alpha Lab'),
      organizationOrder(2, 'Beta Corp'),
    ]

    render(
      <BillingHistoryDialog
        open
        onOpenChange={vi.fn()}
        target='personal'
        organizationName='Current Admin Organization'
      />
    )

    expect(screen.getByText('Organization wallet: Alpha Lab')).toBeVisible()
    expect(screen.getByText('Organization wallet: Beta Corp')).toBeVisible()
    expect(
      screen.queryByText('Organization wallet: Current Admin Organization')
    ).not.toBeInTheDocument()
  })

  test('falls back to the current organization only in its dedicated history', () => {
    mocks.records = [organizationOrder(3)]

    render(
      <BillingHistoryDialog
        open
        onOpenChange={vi.fn()}
        target='organization'
        organizationName='Research Team'
      />
    )

    expect(screen.getByText('Organization wallet: Research Team')).toBeVisible()
  })

  test('marks legacy records unknown and does not offer manual completion', () => {
    mocks.records = [
      {
        ...organizationOrder(4),
        topup_target: undefined,
        status: 'pending',
      },
    ]

    render(
      <BillingHistoryDialog open onOpenChange={vi.fn()} target='personal' />
    )

    expect(screen.getByText('Unknown')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Complete Order' })
    ).not.toBeInTheDocument()
  })
})
