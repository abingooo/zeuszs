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
import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import type { AmountResponse } from '../types'
import { usePayment } from './use-payment'

const mocks = vi.hoisted(() => ({
  calculateAmount: vi.fn(),
}))

vi.mock('../api', () => ({
  calculateAmount: mocks.calculateAmount,
  calculateStripeAmount: vi.fn(),
  calculateWaffoAmount: vi.fn(),
  calculateWaffoPancakeAmount: vi.fn(),
  requestPayment: vi.fn(),
  requestStripePayment: vi.fn(),
  isApiSuccess: (response: { success?: boolean; message?: string }) =>
    response.success === true || response.message === 'success',
}))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('payment amount request ordering', () => {
  test('a late calculation cannot overwrite the newest target and amount', async () => {
    let resolveFirst!: (response: AmountResponse) => void
    let resolveSecond!: (response: AmountResponse) => void
    mocks.calculateAmount
      .mockImplementationOnce(
        () =>
          new Promise<AmountResponse>((resolve) => {
            resolveFirst = resolve
          })
      )
      .mockImplementationOnce(
        () =>
          new Promise<AmountResponse>((resolve) => {
            resolveSecond = resolve
          })
      )

    const { result } = renderHook(() => usePayment())
    let firstRequest!: ReturnType<typeof result.current.calculatePaymentAmount>
    let secondRequest!: ReturnType<typeof result.current.calculatePaymentAmount>

    act(() => {
      firstRequest = result.current.calculatePaymentAmount(
        10,
        'alipay',
        'personal'
      )
      secondRequest = result.current.calculatePaymentAmount(
        25,
        'alipay',
        'organization'
      )
    })

    expect(mocks.calculateAmount).toHaveBeenNthCalledWith(1, {
      amount: 10,
      topup_target: 'personal',
    })
    expect(mocks.calculateAmount).toHaveBeenNthCalledWith(2, {
      amount: 25,
      topup_target: 'organization',
    })

    let secondResult
    await act(async () => {
      resolveSecond({ success: true, data: '8.50' })
      secondResult = await secondRequest
    })
    expect(secondResult).toEqual({ status: 'success', amount: 8.5 })
    expect(result.current.amount).toBe(8.5)
    expect(result.current.calculating).toBe(false)

    let firstResult
    await act(async () => {
      resolveFirst({ success: true, data: '4.00' })
      firstResult = await firstRequest
    })
    expect(firstResult).toEqual({ status: 'stale' })
    expect(result.current.amount).toBe(8.5)
    await waitFor(() => expect(result.current.calculating).toBe(false))
  })
})
