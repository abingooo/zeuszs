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
import { useMutation } from '@tanstack/react-query'
import { useMemo } from 'react'
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
import { getCurrencyLabel } from '@/lib/currency'
import {
  formatQuota,
  getEditableQuotaStep,
  parseQuotaFromDollars,
} from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import { creditOrganizationFund } from '../api'

type FundCreditFormValues = {
  amount: number
  reference: string
}

type CreditOrganizationFundDialogProps = {
  organizationId: number
  organizationName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CreditOrganizationFundDialog(
  props: CreditOrganizationFundDialogProps
) {
  const { t } = useTranslation()
  const currencyLabel = getCurrencyLabel()
  const quotaStep = getEditableQuotaStep()
  const schema = useMemo(
    () =>
      z.object({
        amount: z
          .number()
          .positive(t('Fund amount must be greater than zero'))
          .refine((amount) => {
            const quota = parseQuotaFromDollars(amount)
            return quota > 0 && Number.isSafeInteger(quota)
          }, t('Fund amount is outside the supported range')),
        reference: z.string().trim().max(128),
      }),
    [t]
  )
  const form = useForm<FundCreditFormValues>({
    resolver: zodResolver(schema),
    defaultValues: { amount: 0, reference: '' },
  })
  const creditMutation = useMutation({
    mutationFn: (values: FundCreditFormValues) =>
      creditOrganizationFund(props.organizationId, {
        amount: parseQuotaFromDollars(values.amount),
        reference: values.reference || undefined,
      }),
    onSuccess: (result) => {
      toast.success(
        t('Organization pool credited. New pool balance: {{amount}}', {
          amount: formatQuota(result.pool_quota_after),
        })
      )
      form.reset({ amount: 0, reference: '' })
      props.onOpenChange(false)
    },
    onError: handleServerError,
  })

  const handleOpenChange = (open: boolean) => {
    if (creditMutation.isPending) return
    if (!open) form.reset({ amount: 0, reference: '' })
    props.onOpenChange(open)
  }

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Credit organization pool')}</DialogTitle>
          <DialogDescription>
            {t(
              'Add budget to {{organization}} from a verified external receipt or manual adjustment.',
              {
                organization: props.organizationName,
              }
            )}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            id='credit-organization-fund-form'
            onSubmit={form.handleSubmit((values) =>
              creditMutation.mutate(values)
            )}
          >
            <fieldset disabled={creditMutation.isPending}>
              <FieldGroup>
                <FormField
                  control={form.control}
                  name='amount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('Pool credit amount ({{currency}})', {
                          currency: currencyLabel,
                        })}
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min={quotaStep}
                          step={quotaStep}
                          onChange={(event) =>
                            field.onChange(event.target.valueAsNumber)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'This credits the organization budget pool only. It does not change any user balance.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='reference'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Reference')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder={t(
                            'External receipt or adjustment reference'
                          )}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Optional reference for audit and reconciliation.')}
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
            form='credit-organization-fund-form'
            disabled={creditMutation.isPending}
          >
            {creditMutation.isPending && <Spinner data-icon='inline-start' />}
            {creditMutation.isPending ? t('Crediting...') : t('Credit pool')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
