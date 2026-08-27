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
import axios from 'axios'
import i18next from 'i18next'
import { useState, useCallback, useRef } from 'react'
import { toast } from 'sonner'

import {
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoAmount,
  calculateWaffoPancakeAmount,
  requestPayment,
  requestStripePayment,
  isApiSuccess,
} from '../api'
import {
  isStripePayment,
  isWaffoPayment,
  isWaffoPancakePayment,
  submitPaymentForm,
} from '../lib'
import type { AmountRequest, AmountResponse, TopUpTarget } from '../types'

// ============================================================================
// Payment Hook
// ============================================================================

type AmountCalculator = (request: AmountRequest) => Promise<AmountResponse>

export interface PaymentAmountCalculators {
  regular: AmountCalculator
  stripe: AmountCalculator
  waffo: AmountCalculator
  waffoPancake: AmountCalculator
}

const defaultPaymentAmountCalculators: PaymentAmountCalculators = {
  regular: calculateAmount,
  stripe: calculateStripeAmount,
  waffo: calculateWaffoAmount,
  waffoPancake: calculateWaffoPancakeAmount,
}

export type PaymentCalculationResult =
  | { status: 'success'; amount: number }
  | { status: 'failed'; permissionDenied: boolean }
  | { status: 'stale' }

export async function requestPaymentAmount(
  topupAmount: number,
  paymentType: string,
  topUpTarget: TopUpTarget,
  calculators: PaymentAmountCalculators = defaultPaymentAmountCalculators
): Promise<PaymentCalculationResult> {
  let calculator = calculators.regular
  if (isStripePayment(paymentType)) {
    calculator = calculators.stripe
  } else if (isWaffoPayment(paymentType)) {
    calculator = calculators.waffo
  } else if (isWaffoPancakePayment(paymentType)) {
    calculator = calculators.waffoPancake
  }

  try {
    const response = await calculator({
      amount: topupAmount,
      topup_target: topUpTarget,
    })
    if (!isApiSuccess(response) || !response.data) {
      return { status: 'failed', permissionDenied: false }
    }

    const amount = Number.parseFloat(response.data)
    if (!Number.isFinite(amount) || amount <= 0) {
      return { status: 'failed', permissionDenied: false }
    }

    return { status: 'success', amount }
  } catch (error) {
    return {
      status: 'failed',
      permissionDenied:
        axios.isAxiosError(error) && error.response?.status === 403,
    }
  }
}

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)
  const calculationRequestIdRef = useRef(0)

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (
      topupAmount: number,
      paymentType: string,
      topUpTarget: TopUpTarget
    ) => {
      const requestId = ++calculationRequestIdRef.current
      try {
        setCalculating(true)
        const result = await requestPaymentAmount(
          topupAmount,
          paymentType,
          topUpTarget
        )

        if (requestId !== calculationRequestIdRef.current) {
          return { status: 'stale' } as const
        }

        setAmount(result.status === 'success' ? result.amount : 0)
        return result
      } finally {
        if (requestId === calculationRequestIdRef.current) {
          setCalculating(false)
        }
      }
    },
    []
  )

  // Process payment
  const processPayment = useCallback(
    async (
      topupAmount: number,
      paymentType: string,
      topUpTarget: TopUpTarget
    ) => {
      try {
        setProcessing(true)

        const isStripe = isStripePayment(paymentType)
        const amount = Math.floor(topupAmount)

        const response = isStripe
          ? await requestStripePayment({
              amount,
              payment_method: 'stripe',
              topup_target: topUpTarget,
            })
          : await requestPayment({
              amount,
              payment_method: paymentType,
              topup_target: topUpTarget,
            })

        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return false
        }

        // Handle Stripe payment
        if (isStripe && response.data?.pay_link) {
          window.open(response.data.pay_link as string, '_blank')
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        // Handle non-Stripe payment
        if (!isStripe && response.data) {
          const url = (response as unknown as { url?: string }).url
          if (url) {
            submitPaymentForm(url, response.data)
            toast.success(i18next.t('Redirecting to payment page...'))
            return true
          }
        }

        return false
      } catch {
        toast.error(i18next.t('Payment request failed'))
        return false
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  return {
    amount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    setAmount,
  }
}
