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
import { MoneyAdd01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLegend,
  FieldSet,
  FieldTitle,
} from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  organizationKeys,
  updateOrganizationStatus,
  updateOrganizationTopupPolicy,
} from '../api'
import type { Organization, OrganizationStatus } from '../types'
import { CreditOrganizationFundDialog } from './credit-organization-fund-dialog'

type OrganizationSettingsPanelProps = {
  organization: Organization
  onOrganizationChange: (organization: Organization) => void
}

export function OrganizationSettingsPanel(
  props: OrganizationSettingsPanelProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedStatus, setSelectedStatus] = useState(
    props.organization.status
  )
  const [fundDialogOpen, setFundDialogOpen] = useState(false)
  const statusItems = useMemo(
    () => [
      { value: 'active', label: t('Active') },
      { value: 'disabled', label: t('Disabled') },
      { value: 'dissolving', label: t('Dissolving') },
      { value: 'dissolved', label: t('Dissolved') },
    ],
    [t]
  )
  const topupMutation = useMutation({
    mutationFn: (allowMemberTopup: boolean) =>
      updateOrganizationTopupPolicy(props.organization.id, allowMemberTopup),
    onSuccess: (organization) => {
      void queryClient.invalidateQueries({ queryKey: organizationKeys.lists() })
      props.onOrganizationChange({
        ...props.organization,
        ...organization,
      })
      toast.success(t('Member top-up policy updated'))
    },
    onError: handleServerError,
  })
  const statusMutation = useMutation({
    mutationFn: (status: OrganizationStatus) =>
      updateOrganizationStatus(props.organization.id, status),
    onSuccess: (organization) => {
      void queryClient.invalidateQueries({ queryKey: organizationKeys.lists() })
      props.onOrganizationChange({
        ...props.organization,
        ...organization,
      })
      toast.success(t('Organization status updated'))
    },
    onError: handleServerError,
  })

  return (
    <div className='flex flex-col gap-6'>
      <FieldSet>
        <FieldLegend>{t('Organization profile')}</FieldLegend>
        <FieldGroup>
          <dl className='grid gap-4 sm:grid-cols-2'>
            <div>
              <dt className='text-muted-foreground text-xs'>{t('Owner')}</dt>
              <dd className='mt-1 font-medium'>
                {props.organization.owner_username ||
                  t('User #{{id}}', {
                    id: props.organization.owner_user_id,
                  })}
              </dd>
            </div>
            <div>
              <dt className='text-muted-foreground text-xs'>{t('Created')}</dt>
              <dd className='mt-1 font-medium'>
                {formatTimestampToDate(props.organization.created_at)}
              </dd>
            </div>
          </dl>
        </FieldGroup>
      </FieldSet>

      <Separator />

      <FieldSet>
        <FieldLegend>{t('Organization policy')}</FieldLegend>
        <FieldGroup>
          <Field orientation='horizontal'>
            <FieldContent>
              <FieldTitle>{t('Allow member top-ups')}</FieldTitle>
              <FieldDescription>
                {t(
                  'Controls whether ordinary members may add funds to their own single balance.'
                )}
              </FieldDescription>
            </FieldContent>
            <Switch
              checked={props.organization.allow_member_topup}
              disabled={topupMutation.isPending}
              onCheckedChange={(checked) => topupMutation.mutate(checked)}
              aria-label={t('Allow member top-ups')}
            />
          </Field>
        </FieldGroup>
      </FieldSet>

      <Separator />

      <FieldSet>
        <FieldLegend>{t('Lifecycle status')}</FieldLegend>
        <FieldDescription>
          {t(
            'Disabled organizations cannot authenticate or use API keys until reactivated.'
          )}
        </FieldDescription>
        <FieldGroup>
          <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
            <Select
              items={statusItems}
              value={selectedStatus}
              disabled={statusMutation.isPending}
              onValueChange={(value) =>
                setSelectedStatus(value as OrganizationStatus)
              }
            >
              <SelectTrigger
                className='w-full sm:w-52'
                aria-label={t('Organization status')}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {statusItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Button
              type='button'
              variant='outline'
              disabled={
                statusMutation.isPending ||
                selectedStatus === props.organization.status
              }
              onClick={() => statusMutation.mutate(selectedStatus)}
            >
              {statusMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Update status')}
            </Button>
          </div>
        </FieldGroup>
      </FieldSet>

      <Separator />

      <FieldSet>
        <FieldLegend>{t('Organization budget pool')}</FieldLegend>
        <FieldDescription>
          {t(
            'Record a verified external receipt or manual adjustment in the organization pool ledger.'
          )}
        </FieldDescription>
        <FieldGroup>
          <div>
            <Button type='button' onClick={() => setFundDialogOpen(true)}>
              <HugeiconsIcon
                icon={MoneyAdd01Icon}
                strokeWidth={2}
                data-icon='inline-start'
              />
              {t('Credit pool')}
            </Button>
          </div>
        </FieldGroup>
      </FieldSet>

      <CreditOrganizationFundDialog
        organizationId={props.organization.id}
        organizationName={props.organization.name}
        open={fundDialogOpen}
        onOpenChange={setFundDialogOpen}
      />
    </div>
  )
}
