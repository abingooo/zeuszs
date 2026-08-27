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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import type { TenantOrganizationSummary } from '@/features/organizations/types'
import { getSelf } from '@/lib/api'

import { Wallet } from '../index'
import type { PaymentMethod, TopUpTarget } from '../types'

const mocks = vi.hoisted(() => ({
  calculatePaymentAmount: vi.fn(),
  processPayment: vi.fn(),
  processWaffoPayment: vi.fn(),
  refetchTopupInfo: vi.fn(),
  invalidateQueries: vi.fn(),
  warning: vi.fn(),
  organizationSummary: null as TenantOrganizationSummary | null,
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: vi.fn(),
  useQueryClient: () => ({ invalidateQueries: mocks.invalidateQueries }),
}))

vi.mock('sonner', () => ({
  toast: {
    warning: mocks.warning,
  },
}))

vi.mock('@/components/layout', () => {
  const SectionPageLayout = Object.assign(
    (props: PropsWithChildren) => <section>{props.children}</section>,
    {
      Title: (props: PropsWithChildren) => <h1>{props.children}</h1>,
      Content: (props: PropsWithChildren) => <div>{props.children}</div>,
    }
  )

  return { SectionPageLayout }
})

vi.mock('@/features/organizations/tenant-api', () => ({
  getTenantOrganizationSummary: vi.fn(),
  tenantOrganizationKeys: {
    summary: () => ['tenant-organization', 'summary'],
  },
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { price: 1 } }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    currency: { quotaDisplayType: 'USD', usdExchangeRate: 1 },
  }),
}))

vi.mock('@/lib/api', () => ({
  getSelf: vi.fn(),
}))

vi.mock('../hooks', () => ({
  useTopupInfo: () => ({
    topupInfo: {
      enable_online_topup: true,
      enable_stripe_topup: false,
      pay_methods: [{ name: 'Card', type: 'card' }],
      min_topup: 10,
      stripe_min_topup: 10,
      amount_options: [],
      discount: {},
      topup_targets: {
        personal: { enabled: true },
        organization: { enabled: true, organization_id: 17 },
      },
    },
    presetAmounts: [],
    loading: false,
    refetch: mocks.refetchTopupInfo,
  }),
  usePayment: () => ({
    amount: 10,
    calculating: false,
    processing: false,
    calculatePaymentAmount: mocks.calculatePaymentAmount,
    processPayment: mocks.processPayment,
  }),
  useAffiliate: () => ({
    affiliateLink: '',
    loading: false,
    transferQuota: vi.fn(),
    transferring: false,
  }),
  useRedemption: () => ({ redeeming: false, redeemCode: vi.fn() }),
  useCreemPayment: () => ({
    processing: false,
    processCreemPayment: vi.fn(),
  }),
  useWaffoPayment: () => ({
    processing: false,
    processWaffoPayment: mocks.processWaffoPayment,
  }),
  useWaffoPancakePayment: () => ({
    processing: false,
    processWaffoPancakePayment: vi.fn(),
  }),
}))

vi.mock('../components/recharge-form-card', () => ({
  RechargeFormCard: (props: {
    topUpTarget: TopUpTarget
    organizationTarget?: { name: string; quota: number }
    onTopUpTargetChange: (target: TopUpTarget) => void
    onTopupAmountChange: (amount: number) => void
    onPaymentMethodSelect: (method: PaymentMethod) => void
  }) => (
    <div>
      <span data-testid='current-target'>{props.topUpTarget}</span>
      <button
        type='button'
        disabled={!props.organizationTarget}
        onClick={() => props.onTopUpTargetChange('organization')}
      >
        Select organization
      </button>
      <button
        type='button'
        onClick={() =>
          props.onPaymentMethodSelect({ name: 'Card', type: 'card' })
        }
      >
        Pay
      </button>
      <button type='button' onClick={() => props.onTopupAmountChange(25)}>
        Set amount 25
      </button>
    </div>
  ),
}))

vi.mock('../components/dialogs/payment-confirm-dialog', () => ({
  PaymentConfirmDialog: (props: {
    open: boolean
    onConfirm: () => void
    topUpTarget: TopUpTarget
    organizationName?: string
    topupAmount: number
    paymentAmount: number
    paymentMethod?: PaymentMethod
  }) => {
    if (!props.open) return null

    return (
      <div role='dialog'>
        <span>{props.topUpTarget}</span>
        <span>{props.organizationName}</span>
        <span data-testid='payment-snapshot'>
          {props.topupAmount}:{props.paymentAmount}:{props.paymentMethod?.type}
        </span>
        <button type='button' onClick={props.onConfirm}>
          Confirm
        </button>
      </div>
    )
  },
}))

vi.mock('../components/affiliate-rewards-card', () => ({
  AffiliateRewardsCard: () => null,
}))

vi.mock('../components/dialogs/billing-history-dialog', () => ({
  BillingHistoryDialog: () => null,
}))

vi.mock('../components/dialogs/creem-confirm-dialog', () => ({
  CreemConfirmDialog: () => null,
}))

vi.mock('../components/dialogs/transfer-dialog', () => ({
  TransferDialog: () => null,
}))

vi.mock('../components/subscription-plans-card', () => ({
  SubscriptionPlansCard: () => null,
}))

vi.mock('../components/wallet-stats-card', () => ({
  WalletStatsCard: () => null,
}))

const ownerSummary: TenantOrganizationSummary = {
  organization_id: 17,
  name: 'Research Team',
  is_default: false,
  status: 'active',
  current_user_role: 'owner',
  member_status: 'active',
  allow_member_topup: false,
}

function setOrganizationRole(role: 'owner' | 'member'): void {
  mocks.organizationSummary = { ...ownerSummary, current_user_role: role }
}

async function selectOrganizationTarget(): Promise<void> {
  fireEvent.click(screen.getByRole('button', { name: 'Select organization' }))
  await waitFor(() =>
    expect(screen.getByTestId('current-target')).toHaveTextContent(
      'organization'
    )
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  setOrganizationRole('owner')
  vi.mocked(useQuery).mockImplementation(
    () => ({ data: mocks.organizationSummary }) as ReturnType<typeof useQuery>
  )
  vi.mocked(getSelf).mockResolvedValue({
    success: true,
    data: {
      id: 1,
      username: 'owner',
      quota: 100,
      used_quota: 0,
      request_count: 0,
      aff_quota: 0,
      aff_history_quota: 0,
      aff_count: 0,
      group: 'default',
    },
  })
  mocks.calculatePaymentAmount.mockResolvedValue({
    status: 'success',
    amount: 10,
  })
  mocks.processPayment.mockResolvedValue(true)
  mocks.processWaffoPayment.mockResolvedValue(true)
  mocks.refetchTopupInfo.mockResolvedValue(undefined)
  mocks.invalidateQueries.mockResolvedValue(undefined)
})

describe('wallet payment target authorization', () => {
  test('closes an open organization payment when the payer loses organization authority', async () => {
    const view = render(<Wallet />)
    await selectOrganizationTarget()

    fireEvent.click(screen.getByRole('button', { name: 'Pay' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent(
      'organizationResearch Team'
    )

    setOrganizationRole('member')
    view.rerender(<Wallet />)

    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(screen.getByTestId('current-target')).toHaveTextContent('personal')
    expect(mocks.processPayment).not.toHaveBeenCalled()
    expect(mocks.warning).toHaveBeenCalledOnce()
  })

  test('does not reopen an organization payment after authority is lost during amount calculation', async () => {
    const view = render(<Wallet />)
    await selectOrganizationTarget()
    await waitFor(() => expect(mocks.calculatePaymentAmount).toHaveBeenCalled())
    mocks.calculatePaymentAmount.mockClear()

    let resolveCalculation: () => void = () => undefined
    mocks.calculatePaymentAmount.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveCalculation = () =>
            resolve({ status: 'success' as const, amount: 10 })
        })
    )

    fireEvent.click(screen.getByRole('button', { name: 'Pay' }))
    await waitFor(() =>
      expect(mocks.calculatePaymentAmount).toHaveBeenCalledOnce()
    )

    setOrganizationRole('member')
    view.rerender(<Wallet />)
    await waitFor(() =>
      expect(screen.getByTestId('current-target')).toHaveTextContent('personal')
    )

    await act(async () => resolveCalculation())

    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(mocks.processPayment).not.toHaveBeenCalled()
    expect(mocks.warning).toHaveBeenCalledOnce()
  })

  test('submits an authorized organization payment with the captured target', async () => {
    render(<Wallet />)
    await selectOrganizationTarget()

    fireEvent.click(screen.getByRole('button', { name: 'Pay' }))
    await screen.findByRole('dialog')
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))

    await waitFor(() =>
      expect(mocks.processPayment).toHaveBeenCalledWith(
        10,
        'card',
        'organization'
      )
    )
  })

  test('submits a personal payment with the captured amount, method, and price', async () => {
    mocks.calculatePaymentAmount.mockResolvedValueOnce({
      status: 'success',
      amount: 7.25,
    })
    render(<Wallet />)
    await waitFor(() => expect(mocks.calculatePaymentAmount).toHaveBeenCalled())
    mocks.calculatePaymentAmount.mockResolvedValueOnce({
      status: 'success',
      amount: 7.25,
    })

    fireEvent.click(screen.getByRole('button', { name: 'Pay' }))
    expect(await screen.findByRole('dialog')).toBeVisible()
    expect(screen.getByTestId('payment-snapshot')).toHaveTextContent(
      '10:7.25:card'
    )

    fireEvent.click(screen.getByRole('button', { name: 'Set amount 25' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))

    await waitFor(() =>
      expect(mocks.processPayment).toHaveBeenCalledWith(10, 'card', 'personal')
    )
  })

  test('does not open confirmation when amount calculation fails', async () => {
    render(<Wallet />)
    await waitFor(() => expect(mocks.calculatePaymentAmount).toHaveBeenCalled())
    mocks.calculatePaymentAmount.mockResolvedValueOnce({
      status: 'failed',
      permissionDenied: false,
    })

    fireEvent.click(screen.getByRole('button', { name: 'Pay' }))

    await waitFor(() =>
      expect(mocks.calculatePaymentAmount).toHaveBeenLastCalledWith(
        10,
        'card',
        'personal'
      )
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  test('refreshes authorization after organization calculation is rejected', async () => {
    render(<Wallet />)
    await selectOrganizationTarget()
    await waitFor(() => expect(mocks.calculatePaymentAmount).toHaveBeenCalled())
    mocks.calculatePaymentAmount.mockResolvedValueOnce({
      status: 'failed',
      permissionDenied: true,
    })

    fireEvent.click(screen.getByRole('button', { name: 'Pay' }))

    await waitFor(() => expect(mocks.refetchTopupInfo).toHaveBeenCalledOnce())
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['tenant-organization', 'summary'],
    })
    expect(screen.getByTestId('current-target')).toHaveTextContent('personal')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mocks.warning).toHaveBeenCalledOnce()
  })
})
