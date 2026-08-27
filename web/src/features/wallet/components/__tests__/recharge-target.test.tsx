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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import type { TopupInfo } from '../../types'
import { RechargeFormCard } from '../recharge-form-card'

const topupInfo: TopupInfo = {
  enable_online_topup: true,
  enable_stripe_topup: false,
  pay_methods: [],
  min_topup: 10,
  stripe_min_topup: 10,
  amount_options: [],
  discount: {},
  enable_redemption: true,
}

const baseProps = {
  topupInfo,
  presetAmounts: [],
  selectedPreset: null,
  onSelectPreset: vi.fn(),
  topupAmount: 10,
  onTopupAmountChange: vi.fn(),
  paymentAmount: 10,
  calculating: false,
  onPaymentMethodSelect: vi.fn(),
  paymentLoading: null,
  redemptionCode: '',
  onRedemptionCodeChange: vi.fn(),
  onRedeem: vi.fn(),
  redeeming: false,
}

describe('wallet recharge target', () => {
  test('does not expose organization wallet controls to an ordinary member', () => {
    render(
      <RechargeFormCard
        {...baseProps}
        topUpTarget='personal'
        onTopUpTargetChange={vi.fn()}
      />
    )

    expect(screen.queryByText('Organization wallet')).not.toBeInTheDocument()
    expect(screen.getByText('Have a Code?')).toBeInTheDocument()
  })

  test('lets an authorized manager select the organization wallet without exposing personal redemption', () => {
    const onTopUpTargetChange = vi.fn()
    render(
      <RechargeFormCard
        {...baseProps}
        topUpTarget='organization'
        onTopUpTargetChange={onTopUpTargetChange}
        organizationTarget={{ name: 'Research Team', quota: 5000 }}
      />
    )

    expect(screen.getByText('Organization wallet')).toBeInTheDocument()
    expect(screen.getByText(/Organization balance/)).toBeInTheDocument()
    expect(screen.queryByText('Have a Code?')).not.toBeInTheDocument()

    fireEvent.click(screen.getByText('Personal wallet'))
    expect(onTopUpTargetChange).toHaveBeenCalledWith('personal')
  })
})
