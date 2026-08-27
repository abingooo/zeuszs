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
import { beforeEach, describe, expect, test, vi } from 'vitest'

import {
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoAmount,
  calculateWaffoPancakeAmount,
  completeOrder,
  getUserBillingHistory,
  requestCreemPayment,
  requestPayment,
  requestStripePayment,
  requestWaffoPayment,
  requestWaffoPancakePayment,
} from '../api'
import type { TopUpTarget } from '../types'

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    get: mocks.get,
    post: mocks.post,
  },
}))

const paymentCases: Array<{
  name: string
  personalPath: string
  organizationPath: string
  invoke: (target: TopUpTarget) => Promise<unknown>
  body: Record<string, unknown>
}> = [
  {
    name: 'regular amount',
    personalPath: '/api/user/amount',
    organizationPath: '/api/organization/topup/amount',
    invoke: (topup_target) => calculateAmount({ amount: 20, topup_target }),
    body: { amount: 20 },
  },
  {
    name: 'regular payment',
    personalPath: '/api/user/pay',
    organizationPath: '/api/organization/topup/pay',
    invoke: (topup_target) =>
      requestPayment({ amount: 20, payment_method: 'epay', topup_target }),
    body: { amount: 20, payment_method: 'epay' },
  },
  {
    name: 'Stripe amount',
    personalPath: '/api/user/stripe/amount',
    organizationPath: '/api/organization/topup/stripe/amount',
    invoke: (topup_target) =>
      calculateStripeAmount({ amount: 20, topup_target }),
    body: { amount: 20 },
  },
  {
    name: 'Stripe payment',
    personalPath: '/api/user/stripe/pay',
    organizationPath: '/api/organization/topup/stripe/pay',
    invoke: (topup_target) =>
      requestStripePayment({
        amount: 20,
        payment_method: 'stripe',
        topup_target,
      }),
    body: { amount: 20, payment_method: 'stripe' },
  },
  {
    name: 'Creem payment',
    personalPath: '/api/user/creem/pay',
    organizationPath: '/api/organization/topup/creem/pay',
    invoke: (topup_target) =>
      requestCreemPayment({
        product_id: 'product-1',
        payment_method: 'creem',
        topup_target,
      }),
    body: { product_id: 'product-1', payment_method: 'creem' },
  },
  {
    name: 'Waffo amount',
    personalPath: '/api/user/waffo/amount',
    organizationPath: '/api/organization/topup/waffo/amount',
    invoke: (topup_target) =>
      calculateWaffoAmount({ amount: 20, topup_target }),
    body: { amount: 20 },
  },
  {
    name: 'Waffo payment',
    personalPath: '/api/user/waffo/pay',
    organizationPath: '/api/organization/topup/waffo/pay',
    invoke: (topup_target) =>
      requestWaffoPayment({
        amount: 20,
        pay_method_index: 3,
        topup_target,
      }),
    body: { amount: 20, pay_method_index: 3 },
  },
  {
    name: 'Waffo Pancake amount',
    personalPath: '/api/user/waffo-pancake/amount',
    organizationPath: '/api/organization/topup/waffo-pancake/amount',
    invoke: (topup_target) =>
      calculateWaffoPancakeAmount({ amount: 20, topup_target }),
    body: { amount: 20 },
  },
  {
    name: 'Waffo Pancake payment',
    personalPath: '/api/user/waffo-pancake/pay',
    organizationPath: '/api/organization/topup/waffo-pancake/pay',
    invoke: (topup_target) =>
      requestWaffoPancakePayment({ amount: 20, topup_target }),
    body: { amount: 20 },
  },
]

beforeEach(() => {
  vi.clearAllMocks()
  mocks.get.mockResolvedValue({ data: { success: true, data: {} } })
  mocks.post.mockResolvedValue({ data: { success: true, data: {} } })
})

describe('wallet API target routing', () => {
  test.each(paymentCases)(
    '$name keeps personal and organization routes isolated',
    async ({ personalPath, organizationPath, invoke, body }) => {
      await invoke('personal')
      expect(mocks.post).toHaveBeenLastCalledWith(
        personalPath,
        { ...body, topup_target: 'personal' },
        expect.objectContaining({ skipBusinessError: true })
      )

      await invoke('organization')
      expect(mocks.post).toHaveBeenLastCalledWith(
        organizationPath,
        { ...body, topup_target: 'organization' },
        expect.objectContaining({ skipBusinessError: true })
      )
    }
  )

  test.each([
    ['personal', '/api/user/topup/self'],
    ['organization', '/api/organization/topup/self'],
  ] as const)(
    'loads %s billing history from its own route',
    async (target, path) => {
      await getUserBillingHistory(2, 20, 'order', target)

      expect(mocks.get).toHaveBeenCalledWith(
        `${path}?p=2&page_size=20&keyword=order`
      )
    }
  )

  test.each([
    ['personal', '/api/user/topup/complete'],
    ['organization', '/api/organization/admin/topup/complete'],
  ] as const)(
    'completes %s orders through their own route',
    async (target, path) => {
      await completeOrder({ trade_no: 'trade-1', topup_target: target })

      expect(mocks.post).toHaveBeenCalledWith(path, {
        trade_no: 'trade-1',
        topup_target: target,
      })
    }
  )
})
