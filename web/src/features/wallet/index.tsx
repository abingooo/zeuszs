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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import {
  getTenantOrganizationSummary,
  tenantOrganizationKeys,
} from '@/features/organizations/tenant-api'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'
import { getSelf } from '@/lib/api'

import { AffiliateRewardsCard } from './components/affiliate-rewards-card'
import { BillingHistoryDialog } from './components/dialogs/billing-history-dialog'
import { CreemConfirmDialog } from './components/dialogs/creem-confirm-dialog'
import { PaymentConfirmDialog } from './components/dialogs/payment-confirm-dialog'
import { TransferDialog } from './components/dialogs/transfer-dialog'
import { RechargeFormCard } from './components/recharge-form-card'
import { SubscriptionPlansCard } from './components/subscription-plans-card'
import { WalletStatsCard } from './components/wallet-stats-card'
import { DEFAULT_DISCOUNT_RATE, PAYMENT_TYPES } from './constants'
import {
  useTopupInfo,
  usePayment,
  useAffiliate,
  useRedemption,
  useCreemPayment,
  useWaffoPayment,
  useWaffoPancakePayment,
} from './hooks'
import {
  getDefaultPaymentType,
  getMinTopupAmount,
  dispatchSelectedPayment,
} from './lib'
import type {
  UserWalletData,
  PaymentMethod,
  PresetAmount,
  CreemProduct,
  WaffoPayMethod,
  TopUpTarget,
} from './types'

interface WalletProps {
  initialShowHistory?: boolean
}

interface TopUpTargetSnapshot {
  target: TopUpTarget
  organizationId?: number
  organizationName?: string
}

interface PaymentSnapshot extends TopUpTargetSnapshot {
  topupAmount: number
  paymentAmount: number
  paymentMethod: PaymentMethod
  waffoMethodIndex: number | null
}

interface CreemPaymentSnapshot extends TopUpTargetSnapshot {
  product: CreemProduct
}

interface TopUpTargetContext {
  target: TopUpTarget
  canTopUpOrganization: boolean
  organizationId?: number
}

function isTopUpTargetSnapshotAuthorized(
  snapshot: TopUpTargetSnapshot,
  context: TopUpTargetContext
): boolean {
  if (snapshot.target !== context.target) return false
  if (snapshot.target === 'personal') return true

  return (
    context.canTopUpOrganization &&
    snapshot.organizationId !== undefined &&
    snapshot.organizationId === context.organizationId
  )
}

export function Wallet(props: WalletProps) {
  const { t } = useTranslation()
  const [user, setUser] = useState<UserWalletData | null>(null)
  const [userLoading, setUserLoading] = useState(true)
  const [topupAmount, setTopupAmount] = useState(0)
  const [selectedPreset, setSelectedPreset] = useState<number | null>(null)
  const [selectedPaymentMethod, setSelectedPaymentMethod] =
    useState<PaymentMethod>()
  const [paymentLoading, setPaymentLoading] = useState<string | null>(null)
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false)
  const [transferDialogOpen, setTransferDialogOpen] = useState(false)
  const [billingDialogOpen, setBillingDialogOpen] = useState(false)
  const [redemptionCode, setRedemptionCode] = useState('')
  const [creemDialogOpen, setCreemDialogOpen] = useState(false)
  const [showSubscriptionPanel, setShowSubscriptionPanel] = useState(true)
  const [topUpTarget, setTopUpTarget] = useState<TopUpTarget>('personal')
  const [paymentSnapshot, setPaymentSnapshot] =
    useState<PaymentSnapshot | null>(null)
  const [creemPaymentSnapshot, setCreemPaymentSnapshot] =
    useState<CreemPaymentSnapshot | null>(null)

  const { status } = useStatus()
  const { currency } = useSystemConfig()
  const queryClient = useQueryClient()
  const {
    topupInfo,
    presetAmounts,
    loading: topupLoading,
    refetch: refetchTopupInfo,
  } = useTopupInfo()
  const organizationTopUpId =
    topupInfo?.topup_targets?.organization?.organization_id
  const organizationSummaryQuery = useQuery({
    queryKey: [...tenantOrganizationKeys.summary(), organizationTopUpId],
    queryFn: getTenantOrganizationSummary,
    enabled:
      topupInfo?.topup_targets?.organization?.enabled === true &&
      (organizationTopUpId ?? 0) > 0,
  })
  const organizationSummary =
    organizationSummaryQuery.data?.organization_id === organizationTopUpId
      ? organizationSummaryQuery.data
      : undefined
  const canTopUpOrganization =
    topupInfo?.topup_targets?.organization?.enabled === true &&
    (organizationSummary?.current_user_role === 'owner' ||
      organizationSummary?.current_user_role === 'admin')
  const topUpTargetContextRef = useRef<TopUpTargetContext>({
    target: topUpTarget,
    canTopUpOrganization,
    organizationId: organizationTopUpId,
  })
  topUpTargetContextRef.current = {
    target: topUpTarget,
    canTopUpOrganization,
    organizationId: organizationTopUpId,
  }

  // Calculate effective exchange rate - when display type is USD, use rate of 1
  const effectiveUsdExchangeRate = useMemo(() => {
    return currency?.quotaDisplayType === 'USD'
      ? 1
      : currency?.usdExchangeRate || 1
  }, [currency?.quotaDisplayType, currency?.usdExchangeRate])
  const {
    amount: paymentAmount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
  } = usePayment()
  const {
    affiliateLink,
    loading: affiliateLoading,
    transferQuota,
    transferring,
  } = useAffiliate()
  const { redeeming, redeemCode } = useRedemption()
  const { processing: creemProcessing, processCreemPayment } = useCreemPayment()
  const { processing: waffoProcessing, processWaffoPayment } = useWaffoPayment()
  const { processing: pancakeProcessing, processWaffoPancakePayment } =
    useWaffoPancakePayment()

  // Fetch and refresh user data
  const fetchUser = useCallback(async () => {
    try {
      setUserLoading(true)
      const response = await getSelf()
      if (response.success && response.data) {
        setUser(response.data as UserWalletData)
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to fetch user data:', error)
    } finally {
      setUserLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchUser()
  }, [fetchUser])

  useEffect(() => {
    if (props.initialShowHistory) {
      setBillingDialogOpen(true)
      window.history.replaceState({}, '', window.location.pathname)
    }
  }, [props.initialShowHistory])

  // Initialize topup amount when topup info is loaded
  const topupAmountInitializedRef = useRef(false)
  useEffect(() => {
    if (topupInfo && !topupAmountInitializedRef.current) {
      topupAmountInitializedRef.current = true
      const minTopup = getMinTopupAmount(topupInfo)
      setTopupAmount(minTopup)

      // Calculate initial payment amount with default payment type
      const defaultPaymentType = getDefaultPaymentType(topupInfo)
      calculatePaymentAmount(minTopup, defaultPaymentType, topUpTarget)
    }
  }, [topupInfo, calculatePaymentAmount, topUpTarget])

  const clearPendingPayments = useCallback(() => {
    setSelectedPaymentMethod(undefined)
    setPaymentSnapshot(null)
    setCreemPaymentSnapshot(null)
    setConfirmDialogOpen(false)
    setCreemDialogOpen(false)
  }, [])

  useEffect(() => {
    if (topUpTarget === 'organization' && !canTopUpOrganization) {
      const hadPendingPayment =
        paymentSnapshot?.target === 'organization' ||
        creemPaymentSnapshot?.target === 'organization'

      setTopUpTarget('personal')
      clearPendingPayments()

      if (hadPendingPayment) {
        toast.warning(
          t(
            'Organization recharge permission is no longer available. The pending payment was cancelled.'
          )
        )
      }
    }
  }, [
    canTopUpOrganization,
    clearPendingPayments,
    creemPaymentSnapshot?.target,
    paymentSnapshot?.target,
    t,
    topUpTarget,
  ])

  const captureTopUpTarget = useCallback((): TopUpTargetSnapshot | null => {
    if (topUpTarget === 'personal') {
      return { target: 'personal' }
    }

    if (!canTopUpOrganization || !organizationTopUpId) return null

    return {
      target: 'organization',
      organizationId: organizationTopUpId,
      organizationName: organizationSummary?.name,
    }
  }, [
    canTopUpOrganization,
    organizationSummary?.name,
    organizationTopUpId,
    topUpTarget,
  ])

  const warnAndClearUnauthorizedPayment = useCallback(async () => {
    clearPendingPayments()
    setTopUpTarget('personal')
    toast.warning(
      t(
        'Organization recharge permission is no longer available. The pending payment was cancelled.'
      )
    )
    await Promise.allSettled([
      refetchTopupInfo(),
      queryClient.invalidateQueries({
        queryKey: tenantOrganizationKeys.summary(),
      }),
    ])
  }, [clearPendingPayments, queryClient, refetchTopupInfo, t])

  // Get current payment type (selected or default)
  const getCurrentPaymentType = useCallback(() => {
    return selectedPaymentMethod?.type || getDefaultPaymentType(topupInfo)
  }, [selectedPaymentMethod, topupInfo])

  // Handle preset selection
  const handleSelectPreset = (preset: PresetAmount) => {
    setTopupAmount(preset.value)
    setSelectedPreset(preset.value)
    calculatePaymentAmount(preset.value, getCurrentPaymentType(), topUpTarget)
  }

  // Handle topup amount change
  const handleTopupAmountChange = (amount: number) => {
    setTopupAmount(amount)
    setSelectedPreset(null)
    calculatePaymentAmount(amount, getCurrentPaymentType(), topUpTarget)
  }

  // Handle payment method selection
  const handlePaymentMethodSelect = async (method: PaymentMethod) => {
    const targetSnapshot = captureTopUpTarget()
    if (!targetSnapshot) return

    setSelectedPaymentMethod(method)
    setPaymentLoading(method.type)

    try {
      // Validate minimum topup
      const minTopup = getMinTopupAmount(topupInfo)
      if (topupAmount < minTopup) {
        return
      }

      // Calculate payment amount and show confirmation dialog
      const calculation = await calculatePaymentAmount(
        topupAmount,
        method.type,
        targetSnapshot.target
      )
      if (calculation.status !== 'success') {
        if (
          calculation.status === 'failed' &&
          calculation.permissionDenied &&
          targetSnapshot.target === 'organization'
        ) {
          await warnAndClearUnauthorizedPayment()
        }
        return
      }
      if (
        !isTopUpTargetSnapshotAuthorized(
          targetSnapshot,
          topUpTargetContextRef.current
        )
      ) {
        if (targetSnapshot.target === 'organization') {
          await warnAndClearUnauthorizedPayment()
        }
        return
      }
      setPaymentSnapshot({
        ...targetSnapshot,
        topupAmount,
        paymentAmount: calculation.amount,
        paymentMethod: { ...method },
        waffoMethodIndex: null,
      })
      setConfirmDialogOpen(true)
    } finally {
      setPaymentLoading(null)
    }
  }

  // Handle payment confirmation
  const handlePaymentConfirm = async () => {
    if (!paymentSnapshot) return

    if (
      !isTopUpTargetSnapshotAuthorized(
        paymentSnapshot,
        topUpTargetContextRef.current
      )
    ) {
      if (paymentSnapshot.target === 'organization') {
        await warnAndClearUnauthorizedPayment()
      }
      return
    }

    const success = await dispatchSelectedPayment(
      paymentSnapshot.paymentMethod,
      paymentSnapshot.topupAmount,
      paymentSnapshot.waffoMethodIndex,
      {
        regular: (amount, paymentType) =>
          processPayment(amount, paymentType, paymentSnapshot.target),
        waffo: (amount, methodIndex) =>
          processWaffoPayment(amount, methodIndex, paymentSnapshot.target),
        waffoPancake: (amount) =>
          processWaffoPancakePayment(amount, paymentSnapshot.target),
      }
    )

    if (success) {
      setConfirmDialogOpen(false)
      setPaymentSnapshot(null)
      await fetchUser()
    }
  }

  // Handle redemption
  const handleRedeem = async () => {
    if (!redemptionCode) return

    const success = await redeemCode(redemptionCode)
    if (success) {
      setRedemptionCode('')
      await fetchUser()
    }
  }

  // Handle transfer
  const handleTransfer = async (amount: number) => {
    const success = await transferQuota(amount)
    if (success) {
      await fetchUser()
    }
    return success
  }

  // Handle Creem product selection
  const handleCreemProductSelect = (product: CreemProduct) => {
    const targetSnapshot = captureTopUpTarget()
    if (!targetSnapshot) return

    const productSnapshot = { ...product }
    setCreemPaymentSnapshot({ ...targetSnapshot, product: productSnapshot })
    setCreemDialogOpen(true)
  }

  // Handle Creem payment confirmation
  const handleCreemConfirm = async () => {
    if (!creemPaymentSnapshot) return

    if (
      !isTopUpTargetSnapshotAuthorized(
        creemPaymentSnapshot,
        topUpTargetContextRef.current
      )
    ) {
      if (creemPaymentSnapshot.target === 'organization') {
        await warnAndClearUnauthorizedPayment()
      }
      return
    }

    const success = await processCreemPayment(
      creemPaymentSnapshot.product.productId,
      creemPaymentSnapshot.target
    )
    if (success) {
      setCreemDialogOpen(false)
      setCreemPaymentSnapshot(null)
      await fetchUser()
    }
  }

  const handleWaffoMethodSelect = async (
    method: WaffoPayMethod,
    index: number
  ) => {
    const targetSnapshot = captureTopUpTarget()
    if (!targetSnapshot) return

    const loadingKey = `waffo-${index}`
    const paymentMethod: PaymentMethod = {
      name: method.name,
      type: PAYMENT_TYPES.WAFFO,
      icon: method.icon,
    }
    setSelectedPaymentMethod(paymentMethod)
    setPaymentLoading(loadingKey)

    try {
      const calculation = await calculatePaymentAmount(
        topupAmount,
        PAYMENT_TYPES.WAFFO,
        targetSnapshot.target
      )
      if (calculation.status !== 'success') {
        if (
          calculation.status === 'failed' &&
          calculation.permissionDenied &&
          targetSnapshot.target === 'organization'
        ) {
          await warnAndClearUnauthorizedPayment()
        }
        return
      }
      if (
        !isTopUpTargetSnapshotAuthorized(
          targetSnapshot,
          topUpTargetContextRef.current
        )
      ) {
        if (targetSnapshot.target === 'organization') {
          await warnAndClearUnauthorizedPayment()
        }
        return
      }
      setPaymentSnapshot({
        ...targetSnapshot,
        topupAmount,
        paymentAmount: calculation.amount,
        paymentMethod,
        waffoMethodIndex: index,
      })
      setConfirmDialogOpen(true)
    } finally {
      setPaymentLoading(null)
    }
  }

  // Get discount rate for current topup amount
  const getDiscountRate = useCallback(
    (amount: number) => {
      return topupInfo?.discount?.[amount] || DEFAULT_DISCOUNT_RATE
    },
    [topupInfo]
  )

  const handleSubscriptionAvailabilityChange = useCallback(
    (available: boolean) => {
      setShowSubscriptionPanel(available)
    },
    []
  )

  const handleTopUpTargetChange = useCallback(
    (target: TopUpTarget) => {
      if (target === 'organization' && !canTopUpOrganization) return
      setTopUpTarget(target)
      clearPendingPayments()
      calculatePaymentAmount(topupAmount, getCurrentPaymentType(), target)
    },
    [
      calculatePaymentAmount,
      canTopUpOrganization,
      clearPendingPayments,
      getCurrentPaymentType,
      topupAmount,
    ]
  )

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Wallet')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
            <WalletStatsCard user={user} loading={userLoading} />

            <div
              className={
                showSubscriptionPanel
                  ? 'grid gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(360px,0.95fr)] xl:items-start'
                  : 'grid gap-4'
              }
            >
              <div id='wallet-add-funds' className='scroll-mt-4'>
                <RechargeFormCard
                  topupInfo={topupInfo}
                  presetAmounts={presetAmounts}
                  selectedPreset={selectedPreset}
                  onSelectPreset={handleSelectPreset}
                  topupAmount={topupAmount}
                  onTopupAmountChange={handleTopupAmountChange}
                  paymentAmount={paymentAmount}
                  calculating={calculating}
                  onPaymentMethodSelect={handlePaymentMethodSelect}
                  paymentLoading={paymentLoading}
                  redemptionCode={redemptionCode}
                  onRedemptionCodeChange={setRedemptionCode}
                  onRedeem={handleRedeem}
                  redeeming={redeeming}
                  topupLink={topupInfo?.topup_link}
                  loading={topupLoading}
                  priceRatio={(status?.price as number) || 1}
                  usdExchangeRate={effectiveUsdExchangeRate}
                  onOpenBilling={() => setBillingDialogOpen(true)}
                  creemProducts={topupInfo?.creem_products}
                  enableCreemTopup={topupInfo?.enable_creem_topup}
                  onCreemProductSelect={handleCreemProductSelect}
                  enableWaffoTopup={topupInfo?.enable_waffo_topup}
                  waffoPayMethods={topupInfo?.waffo_pay_methods}
                  waffoMinTopup={topupInfo?.waffo_min_topup}
                  onWaffoMethodSelect={handleWaffoMethodSelect}
                  enableWaffoPancakeTopup={
                    topupInfo?.enable_waffo_pancake_topup
                  }
                  topUpTarget={topUpTarget}
                  onTopUpTargetChange={handleTopUpTargetChange}
                  organizationTarget={
                    canTopUpOrganization && organizationSummary
                      ? {
                          name: organizationSummary.name,
                          quota: organizationSummary.fund_quota,
                        }
                      : undefined
                  }
                />
              </div>

              <SubscriptionPlansCard
                topupInfo={topupInfo}
                onAvailabilityChange={handleSubscriptionAvailabilityChange}
                userQuota={user?.quota}
                onPurchaseSuccess={fetchUser}
              />
            </div>

            <AffiliateRewardsCard
              user={user}
              affiliateLink={affiliateLink}
              onTransfer={() => setTransferDialogOpen(true)}
              complianceConfirmed={
                topupInfo?.payment_compliance_confirmed !== false
              }
              loading={affiliateLoading}
            />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <PaymentConfirmDialog
        open={confirmDialogOpen}
        onOpenChange={(open) => {
          setConfirmDialogOpen(open)
          if (!open) setPaymentSnapshot(null)
        }}
        onConfirm={handlePaymentConfirm}
        topupAmount={paymentSnapshot?.topupAmount ?? 0}
        paymentAmount={paymentSnapshot?.paymentAmount ?? 0}
        paymentMethod={paymentSnapshot?.paymentMethod}
        processing={processing || waffoProcessing || pancakeProcessing}
        discountRate={getDiscountRate(paymentSnapshot?.topupAmount ?? 0)}
        usdExchangeRate={effectiveUsdExchangeRate}
        topUpTarget={paymentSnapshot?.target ?? 'personal'}
        organizationName={paymentSnapshot?.organizationName}
      />

      <TransferDialog
        open={transferDialogOpen}
        onOpenChange={setTransferDialogOpen}
        onConfirm={handleTransfer}
        availableQuota={user?.aff_quota ?? 0}
        transferring={transferring}
      />

      <BillingHistoryDialog
        open={billingDialogOpen}
        onOpenChange={setBillingDialogOpen}
        target={topUpTarget}
        organizationName={organizationSummary?.name}
      />

      <CreemConfirmDialog
        open={creemDialogOpen}
        onOpenChange={(open) => {
          setCreemDialogOpen(open)
          if (!open) {
            setCreemPaymentSnapshot(null)
          }
        }}
        onConfirm={handleCreemConfirm}
        product={creemPaymentSnapshot?.product ?? null}
        processing={creemProcessing}
        topUpTarget={creemPaymentSnapshot?.target ?? 'personal'}
        organizationName={creemPaymentSnapshot?.organizationName}
      />
    </>
  )
}
