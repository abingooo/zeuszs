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
import { Audit01Icon, Invoice03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import {
  listTenantOrganizationAudit,
  listTenantOrganizationLedger,
  tenantOrganizationKeys,
} from '../tenant-api'
import { OrganizationQueryError } from './organization-query-error'
import { PaginationControls } from './pagination-controls'

const PAGE_SIZE = 20

function HistorySkeleton() {
  return (
    <div className='flex flex-col gap-2'>
      {Array.from({ length: 7 }, (_, index) => (
        <Skeleton key={index} className='h-12 w-full' />
      ))}
    </div>
  )
}

export function TenantOrganizationLedgerPanel() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const params = { page, pageSize: PAGE_SIZE }
  const ledgerQuery = useQuery({
    queryKey: tenantOrganizationKeys.ledgerList(params),
    queryFn: () => listTenantOrganizationLedger(params),
    placeholderData: keepPreviousData,
  })
  const entries = ledgerQuery.data?.items ?? []
  const total = ledgerQuery.data?.total ?? 0
  const hasLoadError = ledgerQuery.isError && !ledgerQuery.data

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3'>
      {ledgerQuery.isLoading && <HistorySkeleton />}
      {hasLoadError && (
        <OrganizationQueryError onRetry={() => void ledgerQuery.refetch()} />
      )}
      {!ledgerQuery.isLoading && !hasLoadError && entries.length === 0 && (
        <Empty className='border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={Invoice03Icon} strokeWidth={2} />
            </EmptyMedia>
            <EmptyTitle>{t('No organization ledger entries')}</EmptyTitle>
            <EmptyDescription>
              {t('Quota allocations and recoveries will appear here.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {!ledgerQuery.isLoading && !hasLoadError && entries.length > 0 && (
        <>
          <div className='overflow-x-auto rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Time')}</TableHead>
                  <TableHead>{t('Operation')}</TableHead>
                  <TableHead>{t('Member ID')}</TableHead>
                  <TableHead>{t('User quota change')}</TableHead>
                  <TableHead>{t('Pool quota change')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map((entry) => (
                  <TableRow key={entry.id}>
                    <TableCell>
                      {formatTimestampToDate(entry.created_at)}
                    </TableCell>
                    <TableCell>
                      <code>{entry.operation}</code>
                    </TableCell>
                    <TableCell>{entry.user_id}</TableCell>
                    <TableCell>{formatQuota(entry.user_quota_delta)}</TableCell>
                    <TableCell>{formatQuota(entry.pool_quota_delta)}</TableCell>
                    <TableCell>
                      <Badge variant='outline'>{entry.status}</Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <PaginationControls
            page={page}
            pageSize={PAGE_SIZE}
            total={total}
            disabled={ledgerQuery.isFetching}
            onPageChange={setPage}
          />
        </>
      )}
    </div>
  )
}

export function TenantOrganizationAuditPanel() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const params = { page, pageSize: PAGE_SIZE }
  const auditQuery = useQuery({
    queryKey: tenantOrganizationKeys.auditList(params),
    queryFn: () => listTenantOrganizationAudit(params),
    placeholderData: keepPreviousData,
  })
  const entries = auditQuery.data?.items ?? []
  const total = auditQuery.data?.total ?? 0
  const hasLoadError = auditQuery.isError && !auditQuery.data

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3'>
      {auditQuery.isLoading && <HistorySkeleton />}
      {hasLoadError && (
        <OrganizationQueryError onRetry={() => void auditQuery.refetch()} />
      )}
      {!auditQuery.isLoading && !hasLoadError && entries.length === 0 && (
        <Empty className='border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={Audit01Icon} strokeWidth={2} />
            </EmptyMedia>
            <EmptyTitle>{t('No organization audit events')}</EmptyTitle>
            <EmptyDescription>
              {t('Organization management actions will appear here.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {!auditQuery.isLoading && !hasLoadError && entries.length > 0 && (
        <>
          <div className='overflow-x-auto rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Time')}</TableHead>
                  <TableHead>{t('Action')}</TableHead>
                  <TableHead>{t('Actor user ID')}</TableHead>
                  <TableHead>{t('Initiator user ID')}</TableHead>
                  <TableHead>{t('Target')}</TableHead>
                  <TableHead>{t('Request ID')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map((entry) => (
                  <TableRow key={entry.id}>
                    <TableCell>
                      {formatTimestampToDate(entry.created_at)}
                    </TableCell>
                    <TableCell>
                      <code>{entry.action}</code>
                    </TableCell>
                    <TableCell>{entry.actor_user_id}</TableCell>
                    <TableCell>{entry.initiator_user_id ?? '-'}</TableCell>
                    <TableCell>
                      <span className='inline-block max-w-48 truncate align-middle'>
                        {entry.target_type}:{entry.target_id}
                      </span>
                    </TableCell>
                    <TableCell>
                      <code className='inline-block max-w-40 truncate align-middle'>
                        {entry.request_id}
                      </code>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <PaginationControls
            page={page}
            pageSize={PAGE_SIZE}
            total={total}
            disabled={auditQuery.isFetching}
            onPageChange={setPage}
          />
        </>
      )}
    </div>
  )
}
