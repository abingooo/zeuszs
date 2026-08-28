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
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { useIdVisibility } from '@/hooks/use-id-visibility'
import { formatTimestampToDate } from '@/lib/format'

import {
  getOrganizationLogActionLabel,
  getOrganizationLogActorLabel,
  getOrganizationLogDetails,
  getOrganizationLogTargetLabel,
} from '../../lib/organization-logs'
import type { OrganizationUsageLog } from '../../types'

export function useOrganizationLogsColumns(): ColumnDef<OrganizationUsageLog>[] {
  const { t } = useTranslation()
  const showIDs = useIdVisibility()

  return [
    {
      accessorKey: 'created_at',
      header: t('Time'),
      cell: ({ row }) => (
        <span className='font-mono text-xs tabular-nums'>
          {formatTimestampToDate(row.original.created_at)}
        </span>
      ),
      enableHiding: false,
      size: 180,
    },
    {
      id: 'organization',
      header: t('Organization'),
      accessorFn: (row) => row.organization_name,
      cell: ({ row }) => (
        <div className='flex max-w-[180px] min-w-0 flex-col gap-0.5'>
          <span className='truncate font-medium'>
            {row.original.organization_name || t('Organization')}
          </span>
          {showIDs && row.original.organization_id != null && (
            <span className='text-muted-foreground font-mono text-xs'>
              #{row.original.organization_id}
            </span>
          )}
        </div>
      ),
      size: 180,
    },
    {
      accessorKey: 'action',
      header: t('Action'),
      cell: ({ row }) => (
        <StatusBadge
          label={getOrganizationLogActionLabel(row.original.action, t)}
          autoColor={row.original.action}
          copyable={false}
          showDot={false}
          size='sm'
        />
      ),
      size: 220,
    },
    {
      id: 'actor',
      header: t('Operator'),
      accessorFn: (row) => row.actor_username,
      cell: ({ row }) => {
        const log = row.original
        const actor = getOrganizationLogActorLabel(
          log.actor_username,
          log.actor_user_id ?? 0,
          showIDs,
          t
        )
        const hasInitiator =
          log.initiator_user_id != null || Boolean(log.initiator_username)
        const isSameUser =
          log.initiator_user_id != null && log.actor_user_id != null
            ? log.initiator_user_id === log.actor_user_id
            : Boolean(
                log.initiator_username &&
                log.initiator_username === log.actor_username
              )
        const showInitiator = hasInitiator && !isSameUser

        return (
          <div className='flex max-w-[190px] min-w-0 flex-col gap-0.5'>
            <span className='truncate'>{actor}</span>
            {showInitiator && (
              <span className='text-muted-foreground truncate text-xs'>
                {t('Initiator')}:{' '}
                {getOrganizationLogActorLabel(
                  log.initiator_username || '',
                  log.initiator_user_id || 0,
                  showIDs,
                  t
                )}
              </span>
            )}
          </div>
        )
      },
      size: 190,
    },
    {
      id: 'target',
      header: t('Target'),
      accessorFn: (row) => row.target_name || row.target_type,
      cell: ({ row }) => (
        <span className='block max-w-[180px] truncate'>
          {getOrganizationLogTargetLabel(row.original, showIDs, t)}
        </span>
      ),
      size: 180,
    },
    {
      id: 'details',
      header: t('Details'),
      accessorFn: (row) => getOrganizationLogDetails(row, t),
      cell: ({ row }) => {
        const details = getOrganizationLogDetails(row.original, t)
        return details ? (
          <span className='block max-w-[360px] min-w-[180px] text-sm break-words'>
            {details}
          </span>
        ) : (
          <span className='text-muted-foreground'>-</span>
        )
      },
      size: 300,
    },
    {
      accessorKey: 'request_id',
      header: t('Request ID'),
      cell: ({ row }) => (
        <StatusBadge
          label={row.original.request_id || '-'}
          copyText={row.original.request_id || undefined}
          showDot={false}
          size='sm'
          variant='neutral'
          className='max-w-[180px] font-mono'
        />
      ),
      size: 190,
    },
  ]
}
