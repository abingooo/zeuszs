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
import { Building03Icon, Settings01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatQuota } from '@/lib/format'

import { listOrganizations, organizationKeys } from '../api'
import type { Organization, OrganizationStatus } from '../types'
import { OrganizationStatusBadge } from './organization-badges'
import { OrganizationQueryError } from './organization-query-error'
import { PaginationControls } from './pagination-controls'

const PAGE_SIZE = 20

type OrganizationsListProps = {
  onManage: (organization: Organization) => void
  onCreate: () => void
}

function OrganizationListSkeleton() {
  return (
    <div className='flex flex-1 flex-col gap-2 rounded-lg border p-3'>
      {Array.from({ length: 7 }, (_, index) => (
        <Skeleton key={index} className='h-12 w-full' />
      ))}
    </div>
  )
}

export function OrganizationsList(props: OrganizationsListProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [statusFilter, setStatusFilter] = useState<OrganizationStatus | 'all'>(
    'all'
  )
  const queryParams = {
    page,
    pageSize: PAGE_SIZE,
    status: statusFilter === 'all' ? undefined : statusFilter,
  }
  const organizationsQuery = useQuery({
    queryKey: organizationKeys.list(queryParams),
    queryFn: () => listOrganizations(queryParams),
    placeholderData: keepPreviousData,
  })
  const statusItems = useMemo(
    () => [
      { value: 'all', label: t('All statuses') },
      { value: 'active', label: t('Active') },
      { value: 'disabled', label: t('Disabled') },
      { value: 'dissolving', label: t('Dissolving') },
      { value: 'dissolved', label: t('Dissolved') },
    ],
    [t]
  )

  const organizations = organizationsQuery.data?.items ?? []
  const total = organizationsQuery.data?.total ?? 0
  const hasLoadError = organizationsQuery.isError && !organizationsQuery.data

  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <p className='text-muted-foreground text-sm'>
          {t('Manage organization ownership, roles, and registration access.')}
        </p>
        <Select
          items={statusItems}
          value={statusFilter}
          onValueChange={(value) => {
            setStatusFilter(value as OrganizationStatus | 'all')
            setPage(1)
          }}
        >
          <SelectTrigger className='w-full sm:w-44' aria-label={t('Status')}>
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
      </div>

      {organizationsQuery.isLoading && <OrganizationListSkeleton />}

      {hasLoadError && (
        <OrganizationQueryError
          onRetry={() => void organizationsQuery.refetch()}
        />
      )}

      {!organizationsQuery.isLoading &&
        !hasLoadError &&
        organizations.length === 0 && (
          <Empty className='flex-1 border'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <HugeiconsIcon icon={Building03Icon} strokeWidth={2} />
              </EmptyMedia>
              <EmptyTitle>{t('No organizations found')}</EmptyTitle>
              <EmptyDescription>
                {t('Create an organization and its first Owner account.')}
              </EmptyDescription>
            </EmptyHeader>
            <Button type='button' onClick={props.onCreate}>
              {t('Create organization')}
            </Button>
          </Empty>
        )}

      {!organizationsQuery.isLoading &&
        !hasLoadError &&
        organizations.length > 0 && (
          <>
            <div className='hidden min-h-0 flex-1 overflow-auto rounded-lg border md:block'>
              <Table>
                <TableHeader className='bg-muted/50 sticky top-0'>
                  <TableRow>
                    <TableHead>{t('Organization')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Owner')}</TableHead>
                    <TableHead className='text-right'>{t('Members')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Organization pool balance')}
                    </TableHead>
                    <TableHead>{t('Member top-ups')}</TableHead>
                    <TableHead className='w-12'>
                      <span className='sr-only'>{t('Actions')}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {organizations.map((organization) => (
                    <TableRow key={organization.id}>
                      <TableCell>
                        <div className='flex min-w-52 flex-col gap-0.5'>
                          <span className='max-w-72 truncate font-medium'>
                            {organization.name}
                          </span>
                          <span className='text-muted-foreground text-xs'>
                            {t('ID: {{id}}', { id: organization.id })}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <OrganizationStatusBadge status={organization.status} />
                      </TableCell>
                      <TableCell>
                        <div className='flex flex-col gap-0.5'>
                          <span className='font-medium'>
                            {organization.owner_username ||
                              t('User #{{id}}', {
                                id: organization.owner_user_id,
                              })}
                          </span>
                          <span className='text-muted-foreground text-xs'>
                            {t('User ID: {{id}}', {
                              id: organization.owner_user_id,
                            })}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className='text-right font-medium tabular-nums'>
                        {organization.member_count.toLocaleString()}
                      </TableCell>
                      <TableCell className='text-right font-medium tabular-nums'>
                        {formatQuota(organization.fund_quota)}
                      </TableCell>
                      <TableCell>
                        {organization.allow_member_topup
                          ? t('Allowed')
                          : t('Blocked')}
                      </TableCell>
                      <TableCell>
                        <Tooltip>
                          <TooltipTrigger
                            render={
                              <Button
                                type='button'
                                variant='ghost'
                                size='icon-sm'
                                onClick={() => props.onManage(organization)}
                                aria-label={t('Manage organization')}
                              />
                            }
                          >
                            <HugeiconsIcon
                              icon={Settings01Icon}
                              strokeWidth={2}
                            />
                          </TooltipTrigger>
                          <TooltipContent>
                            {t('Manage organization')}
                          </TooltipContent>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            <div className='min-h-0 flex-1 overflow-auto rounded-lg border md:hidden'>
              <div className='divide-y'>
                {organizations.map((organization) => (
                  <div
                    key={organization.id}
                    className='flex flex-col gap-3 p-3'
                  >
                    <div className='flex min-w-0 items-start justify-between gap-3'>
                      <div className='min-w-0'>
                        <p className='truncate font-medium'>
                          {organization.name}
                        </p>
                        <p className='text-muted-foreground text-xs'>
                          {t('ID: {{id}}', { id: organization.id })}
                        </p>
                      </div>
                      <OrganizationStatusBadge status={organization.status} />
                    </div>
                    <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
                      <div className='min-w-0'>
                        <dt className='text-muted-foreground text-xs'>
                          {t('Owner')}
                        </dt>
                        <dd className='truncate'>
                          {organization.owner_username ||
                            t('User #{{id}}', {
                              id: organization.owner_user_id,
                            })}
                        </dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground text-xs'>
                          {t('Members')}
                        </dt>
                        <dd>{organization.member_count.toLocaleString()}</dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground text-xs'>
                          {t('Organization pool balance')}
                        </dt>
                        <dd>{formatQuota(organization.fund_quota)}</dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground text-xs'>
                          {t('Member top-ups')}
                        </dt>
                        <dd>
                          {organization.allow_member_topup
                            ? t('Allowed')
                            : t('Blocked')}
                        </dd>
                      </div>
                    </dl>
                    <Button
                      type='button'
                      variant='outline'
                      onClick={() => props.onManage(organization)}
                    >
                      <HugeiconsIcon
                        icon={Settings01Icon}
                        strokeWidth={2}
                        data-icon='inline-start'
                      />
                      {t('Manage organization')}
                    </Button>
                  </div>
                ))}
              </div>
            </div>

            <PaginationControls
              page={page}
              pageSize={PAGE_SIZE}
              total={total}
              disabled={organizationsQuery.isFetching}
              onPageChange={setPage}
            />
          </>
        )}
    </div>
  )
}
