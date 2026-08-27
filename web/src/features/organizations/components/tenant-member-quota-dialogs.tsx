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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { FieldGroup } from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { getCurrencyLabel } from '@/lib/currency'
import {
  getEditableQuotaStep,
  parseQuotaFromDollars,
  quotaUnitsToEditableAmount,
} from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  allocateTenantMemberQuota,
  recoverTenantMemberQuota,
  tenantOrganizationKeys,
  updateTenantMemberConsumptionLimit,
} from '../tenant-api'
import type { OrganizationMember } from '../types'

type TenantMemberQuotaDialogProps = {
  member: OrganizationMember
  mode: 'allocate' | 'recover'
  poolQuota: number
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function TenantMemberQuotaDialog(props: TenantMemberQuotaDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currencyLabel = getCurrencyLabel()
  const quotaStep = getEditableQuotaStep()
  const maxQuota =
    props.mode === 'allocate' ? props.poolQuota : props.member.recoverable_quota
  const maxAmount = quotaUnitsToEditableAmount(maxQuota)
  const schema = useMemo(
    () =>
      z.object({
        amount: z
          .number()
          .positive(t('Amount must be greater than zero'))
          .refine((amount) => {
            const quota = parseQuotaFromDollars(amount)
            return quota > 0 && Number.isSafeInteger(quota)
          }, t('Amount is outside the supported range'))
          .refine(
            (amount) => parseQuotaFromDollars(amount) <= maxQuota,
            t('Amount exceeds the available quota')
          ),
      }),
    [maxQuota, t]
  )
  const form = useForm<{ amount: number }>({
    resolver: zodResolver(schema),
    defaultValues: { amount: 0 },
  })
  const mutation = useMutation({
    mutationFn: (values: { amount: number }) => {
      const quota = parseQuotaFromDollars(values.amount)
      if (props.mode === 'allocate') {
        return allocateTenantMemberQuota(props.member.user_id, quota)
      }
      return recoverTenantMemberQuota(props.member.user_id, quota)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: tenantOrganizationKeys.all,
      })
      toast.success(
        props.mode === 'allocate'
          ? t('Quota allocated to member')
          : t('Recoverable quota returned to the organization pool')
      )
      form.reset({ amount: 0 })
      props.onOpenChange(false)
    },
    onError: handleServerError,
  })
  const actionLabel =
    props.mode === 'allocate' ? t('Allocate quota') : t('Recover quota')

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{actionLabel}</DialogTitle>
          <DialogDescription>
            {t('{{action}} for {{username}}.', {
              action: actionLabel,
              username: props.member.username,
            })}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            id='tenant-member-quota-form'
            onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
          >
            <fieldset disabled={mutation.isPending}>
              <FieldGroup>
                <FormField
                  control={form.control}
                  name='amount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('Amount ({{currency}})', {
                          currency: currencyLabel,
                        })}
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min={quotaStep}
                          max={maxAmount}
                          step={quotaStep}
                          onChange={(event) =>
                            field.onChange(event.target.valueAsNumber)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Available: {{amount}} {{currency}}', {
                          amount: maxAmount,
                          currency: currencyLabel,
                        })}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </FieldGroup>
            </fieldset>
          </form>
        </Form>
        <DialogFooter>
          <DialogClose render={<Button variant='outline' />}>
            {t('Cancel')}
          </DialogClose>
          <Button
            type='submit'
            form='tenant-member-quota-form'
            disabled={mutation.isPending}
          >
            {mutation.isPending && <Spinner data-icon='inline-start' />}
            {actionLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type TenantMemberLimitDialogProps = {
  member: OrganizationMember
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function TenantMemberLimitDialog(props: TenantMemberLimitDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currencyLabel = getCurrencyLabel()
  const quotaStep = getEditableQuotaStep()
  const [unlimited, setUnlimited] = useState(
    props.member.consumption_limit === undefined
  )
  const form = useForm<{ amount: number }>({
    resolver: zodResolver(
      z.object({
        amount: z.number().min(0, t('Consumption limit cannot be negative')),
      })
    ),
    defaultValues: {
      amount: quotaUnitsToEditableAmount(props.member.consumption_limit ?? 0),
    },
  })
  const mutation = useMutation({
    mutationFn: (values: { amount: number }) =>
      updateTenantMemberConsumptionLimit(
        props.member.user_id,
        unlimited ? null : parseQuotaFromDollars(values.amount)
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: tenantOrganizationKeys.members(),
      })
      void queryClient.invalidateQueries({
        queryKey: tenantOrganizationKeys.audit(),
      })
      toast.success(t('Member consumption limit updated'))
      props.onOpenChange(false)
    },
    onError: handleServerError,
  })

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Set consumption limit')}</DialogTitle>
          <DialogDescription>
            {t(
              'Set the maximum organization-funded consumption for {{username}}.',
              {
                username: props.member.username,
              }
            )}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            id='tenant-member-limit-form'
            onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
          >
            <fieldset disabled={mutation.isPending}>
              <FieldGroup>
                <FormItem className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                  <div className='flex flex-col gap-1'>
                    <FormLabel htmlFor='member-unlimited'>
                      {t('No consumption limit')}
                    </FormLabel>
                    <FormDescription>
                      {t('The organization pool remains the effective limit.')}
                    </FormDescription>
                  </div>
                  <Switch
                    id='member-unlimited'
                    checked={unlimited}
                    onCheckedChange={setUnlimited}
                  />
                </FormItem>
                <FormField
                  control={form.control}
                  name='amount'
                  render={({ field }) => (
                    <FormItem data-disabled={unlimited}>
                      <FormLabel>
                        {t('Consumption limit ({{currency}})', {
                          currency: currencyLabel,
                        })}
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min={0}
                          step={quotaStep}
                          disabled={unlimited}
                          onChange={(event) =>
                            field.onChange(event.target.valueAsNumber)
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </FieldGroup>
            </fieldset>
          </form>
        </Form>
        <DialogFooter>
          <DialogClose render={<Button variant='outline' />}>
            {t('Cancel')}
          </DialogClose>
          <Button
            type='submit'
            form='tenant-member-limit-form'
            disabled={mutation.isPending}
          >
            {mutation.isPending && <Spinner data-icon='inline-start' />}
            {t('Save limit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
