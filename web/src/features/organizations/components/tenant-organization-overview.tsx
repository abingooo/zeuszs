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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { useIdVisibility } from '@/hooks/use-id-visibility'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import type { TenantOrganizationSummary } from '../types'
import {
  MemberStatusBadge,
  OrganizationRoleBadge,
  OrganizationStatusBadge,
} from './organization-badges'

export function TenantOrganizationOverview(props: {
  summary: TenantOrganizationSummary
}) {
  const { t } = useTranslation()
  const showInternalIds = useIdVisibility()
  const canManage =
    props.summary.current_user_role === 'owner' ||
    props.summary.current_user_role === 'admin'

  return (
    <div className='flex flex-col gap-6'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='min-w-0'>
          <h3 className='truncate text-base font-semibold'>
            {props.summary.name}
          </h3>
          {showInternalIds && (
            <p className='text-muted-foreground mt-1 text-sm'>
              {t('Organization ID: {{id}}', {
                id: props.summary.organization_id,
              })}
            </p>
          )}
        </div>
        <OrganizationStatusBadge status={props.summary.status} />
      </div>

      <dl className='grid overflow-hidden rounded-lg border sm:grid-cols-2 lg:grid-cols-4 [&>div]:min-w-0 [&>div]:border-b [&>div]:p-4 sm:[&>div]:border-r lg:[&>div]:border-b-0'>
        <div>
          <dt className='text-muted-foreground text-xs'>{t('Your role')}</dt>
          <dd className='mt-2'>
            <OrganizationRoleBadge role={props.summary.current_user_role} />
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('Membership status')}
          </dt>
          <dd className='mt-2'>
            <MemberStatusBadge status={props.summary.member_status} />
          </dd>
        </div>
        {canManage && props.summary.member_count !== undefined && (
          <div>
            <dt className='text-muted-foreground text-xs'>{t('Members')}</dt>
            <dd className='mt-1 text-lg font-semibold tabular-nums'>
              {props.summary.member_count.toLocaleString()}
            </dd>
          </div>
        )}
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('Member top-ups')}
          </dt>
          <dd className='mt-2'>
            <Badge
              variant={
                props.summary.allow_member_topup ? 'default' : 'secondary'
              }
            >
              {props.summary.allow_member_topup ? t('Allowed') : t('Blocked')}
            </Badge>
          </dd>
        </div>
      </dl>

      {canManage &&
        props.summary.fund_quota !== undefined &&
        props.summary.created_at !== undefined && (
          <div className='grid gap-4 border-y py-4 sm:grid-cols-2'>
            <div>
              <p className='text-muted-foreground text-xs'>
                {t('Organization pool balance')}
              </p>
              <p className='mt-1 text-lg font-semibold tabular-nums'>
                {formatQuota(props.summary.fund_quota)}
              </p>
            </div>
            <div>
              <p className='text-muted-foreground text-xs'>{t('Created')}</p>
              <p className='mt-1 font-medium'>
                {formatTimestampToDate(props.summary.created_at)}
              </p>
            </div>
          </div>
        )}

      {!canManage && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Your organization administrators manage member access, quotas, and registration policy.'
          )}
        </p>
      )}
    </div>
  )
}
