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
import { describe, expect, test } from 'vitest'

import { PAYMENT_TYPES } from '../constants'
import { requestPaymentAmount } from './use-payment'

describe('payment amount routing', () => {
  test.each([
    [PAYMENT_TYPES.ALIPAY, 'regular'],
    [PAYMENT_TYPES.STRIPE, 'stripe'],
    [PAYMENT_TYPES.WAFFO, 'waffo'],
    [PAYMENT_TYPES.WAFFO_PANCAKE, 'pancake'],
  ])(
    'uses the dedicated %s amount calculator',
    async (paymentType, expected) => {
      const calls: string[] = []
      const result = await requestPaymentAmount(
        120,
        paymentType,
        'organization',
        {
          regular: async (request) => {
            calls.push(`regular:${request.topup_target}`)
            return { success: true, data: '18.75' }
          },
          stripe: async (request) => {
            calls.push(`stripe:${request.topup_target}`)
            return { success: true, data: '18.75' }
          },
          waffo: async (request) => {
            calls.push(`waffo:${request.topup_target}`)
            return { success: true, data: '18.75' }
          },
          waffoPancake: async (request) => {
            calls.push(`pancake:${request.topup_target}`)
            return { success: true, data: '18.75' }
          },
        }
      )

      expect(result).toEqual({ status: 'success', amount: 18.75 })
      expect(calls).toEqual([`${expected}:organization`])
    }
  )

  test('returns a failed result for a business calculation error', async () => {
    const failedCalculator = async () => ({
      success: false,
      message: 'invalid amount',
    })

    const result = await requestPaymentAmount(
      20,
      PAYMENT_TYPES.ALIPAY,
      'personal',
      {
        regular: failedCalculator,
        stripe: failedCalculator,
        waffo: failedCalculator,
        waffoPancake: failedCalculator,
      }
    )

    expect(result).toEqual({ status: 'failed', permissionDenied: false })
  })
})
