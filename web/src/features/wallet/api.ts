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
import { api } from '@/lib/api'

import type {
  RedemptionRequest,
  PaymentRequest,
  AmountRequest,
  AffiliateTransferRequest,
  ApiResponse,
  TopupInfoResponse,
  RedemptionResponse,
  AmountResponse,
  PaymentResponse,
  StripePaymentResponse,
  AffiliateCodeResponse,
  AffiliateTransferResponse,
  BillingHistoryResponse,
  CompleteOrderRequest,
  CreemPaymentRequest,
  CreemPaymentResponse,
  WaffoPaymentRequest,
  WaffoPaymentResponse,
  WaffoPancakePaymentRequest,
  WaffoPancakePaymentResponse,
  TopUpTarget,
} from './types'

// ============================================================================
// Wallet API Functions
// ============================================================================

/**
 * Check if API response is successful
 */
export function isApiSuccess(response: ApiResponse): boolean {
  return response.success === true || response.message === 'success'
}

function getTopUpEndpoint(target: TopUpTarget, path: string): string {
  return target === 'organization'
    ? `/api/organization/topup${path}`
    : `/api/user${path}`
}

/**
 * Get topup configuration info
 */
export async function getTopupInfo(): Promise<TopupInfoResponse> {
  const res = await api.get('/api/user/topup/info')
  return res.data
}

/**
 * Redeem a topup code
 */
export async function redeemTopupCode(
  request: RedemptionRequest
): Promise<RedemptionResponse> {
  const res = await api.post('/api/user/topup', request)
  return res.data
}

/**
 * Calculate payment amount for regular payment
 */
export async function calculateAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post(
    getTopUpEndpoint(request.topup_target, '/amount'),
    request,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Calculate payment amount for Stripe payment
 */
export async function calculateStripeAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post(
    getTopUpEndpoint(request.topup_target, '/stripe/amount'),
    request,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Calculate payment amount for Waffo payment
 */
export async function calculateWaffoAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post(
    getTopUpEndpoint(request.topup_target, '/waffo/amount'),
    request,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Request regular payment
 */
export async function requestPayment(
  request: PaymentRequest
): Promise<PaymentResponse> {
  const res = await api.post(
    getTopUpEndpoint(request.topup_target, '/pay'),
    request,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return {
    ...res.data,
    url: res.data.url || (res as unknown as { url?: string }).url,
  }
}

/**
 * Request Stripe payment
 */
export async function requestStripePayment(
  request: PaymentRequest
): Promise<StripePaymentResponse> {
  const res = await api.post(
    getTopUpEndpoint(request.topup_target, '/stripe/pay'),
    request,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Request Creem payment
 */
export async function requestCreemPayment(
  request: CreemPaymentRequest
): Promise<CreemPaymentResponse> {
  const res = await api.post(
    getTopUpEndpoint(request.topup_target, '/creem/pay'),
    request,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Request Waffo payment
 */
export async function requestWaffoPayment(
  request: WaffoPaymentRequest
): Promise<WaffoPaymentResponse> {
  const res = await api.post(
    getTopUpEndpoint(request.topup_target, '/waffo/pay'),
    request,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Calculate payment amount for Waffo Pancake payment
 */
export async function calculateWaffoPancakeAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post(
    getTopUpEndpoint(request.topup_target, '/waffo-pancake/amount'),
    request,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Request Waffo Pancake payment
 */
export async function requestWaffoPancakePayment(
  request: WaffoPancakePaymentRequest
): Promise<WaffoPancakePaymentResponse> {
  const res = await api.post(
    getTopUpEndpoint(request.topup_target, '/waffo-pancake/pay'),
    request,
    {
      skipBusinessError: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Get affiliate code
 */
export async function getAffiliateCode(): Promise<AffiliateCodeResponse> {
  const res = await api.get('/api/user/aff')
  return res.data
}

/**
 * Transfer affiliate quota to balance
 */
export async function transferAffiliateQuota(
  request: AffiliateTransferRequest
): Promise<AffiliateTransferResponse> {
  const res = await api.post('/api/user/aff_transfer', request)
  return res.data
}

/**
 * Get billing history for current user
 */
export async function getUserBillingHistory(
  page: number,
  pageSize: number,
  keyword?: string,
  target: TopUpTarget = 'personal'
): Promise<ApiResponse<BillingHistoryResponse>> {
  const params = new URLSearchParams({
    p: page.toString(),
    page_size: pageSize.toString(),
  })
  if (keyword) {
    params.append('keyword', keyword)
  }
  const endpoint =
    target === 'organization'
      ? '/api/organization/topup/self'
      : '/api/user/topup/self'
  const res = await api.get(`${endpoint}?${params.toString()}`)
  return res.data
}

/**
 * Get billing history for all users (admin only)
 */
export async function getAllBillingHistory(
  page: number,
  pageSize: number,
  keyword?: string
): Promise<ApiResponse<BillingHistoryResponse>> {
  const params = new URLSearchParams({
    p: page.toString(),
    page_size: pageSize.toString(),
  })
  if (keyword) {
    params.append('keyword', keyword)
  }
  const res = await api.get(`/api/user/topup?${params.toString()}`)
  return res.data
}

/**
 * Complete a pending order (admin only)
 */
export async function completeOrder(
  request: CompleteOrderRequest
): Promise<ApiResponse> {
  const endpoint =
    request.topup_target === 'organization'
      ? '/api/organization/admin/topup/complete'
      : '/api/user/topup/complete'
  const res = await api.post(endpoint, request)
  return res.data
}
