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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePage,
  DataTableRow,
  useDataTable,
} from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { cn } from '@/lib/utils'

import {
  DEFAULT_LOGS_DATA,
  LOG_TYPE_ALL_VALUE,
  LOG_TYPE_ENUM,
  ORGANIZATION_LOG_TYPE_VALUE,
} from '../constants'
import { useColumnsByCategory } from '../lib/columns'
import { parseLogOther } from '../lib/format'
import { fetchLogsByCategory } from '../lib/utils'
import type { LogCategory } from '../types'
import { CommonLogsFilterBar } from './common-logs-filter-bar'
import { TaskLogsFilterBar } from './task-logs-filter-bar'
import { UsageLogsMobileList } from './usage-logs-mobile-card'
import { useLogsViewScope } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const logTypeRowTint: Record<number, string> = {
  [LOG_TYPE_ENUM.ERROR]: 'bg-rose-50/40 dark:bg-rose-950/20',
  [LOG_TYPE_ENUM.REFUND]: 'bg-blue-50/30 dark:bg-blue-950/15',
}

// Warning tint for logs where a quota conversion saturated (admin-only marker).
// Takes precedence over the per-type tint since it flags a billing anomaly.
const quotaSaturationRowTint = 'bg-amber-50/60 dark:bg-amber-950/25'

function getColumnVisibilityStorageKey(
  logCategory: LogCategory,
  isAdmin: boolean
): string {
  return `usage-logs:${logCategory}:${isAdmin ? 'admin' : 'user'}:column-visibility`
}

function deserializeLogTypeFilter(value: unknown): unknown[] {
  let values: unknown[] = []
  if (Array.isArray(value)) {
    values = value
  } else if (value !== undefined && value !== null && value !== '') {
    values = [value]
  }
  return values.filter((item) => String(item) !== LOG_TYPE_ALL_VALUE)
}

interface UsageLogsTableProps {
  logCategory: LogCategory
}

export function UsageLogsTable({ logCategory }: UsageLogsTableProps) {
  const { t } = useTranslation()
  const { canManageScope, isAdminView } = useLogsViewScope()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const searchParams = route.useSearch()
  const organizationFilterSelected =
    Array.isArray(searchParams.type) &&
    searchParams.type.length === 1 &&
    searchParams.type[0] === ORGANIZATION_LOG_TYPE_VALUE
  const effectiveLogCategory: LogCategory =
    logCategory === 'common' && organizationFilterSelected
      ? 'organization'
      : logCategory
  const isAdmin =
    effectiveLogCategory === 'organization' ? canManageScope : isAdminView

  const commonColumnFilters = [
    {
      columnId: 'created_at',
      searchKey: 'type',
      type: 'array' as const,
      deserialize: deserializeLogTypeFilter,
    },
    { columnId: 'model_name', searchKey: 'model', type: 'string' as const },
    { columnId: 'token_name', searchKey: 'token', type: 'string' as const },
    { columnId: 'group', searchKey: 'group', type: 'string' as const },
    ...(isAdmin
      ? [
          {
            columnId: 'channel',
            searchKey: 'channel',
            type: 'string' as const,
          },
          {
            columnId: 'username',
            searchKey: 'username',
            type: 'string' as const,
          },
          {
            columnId: 'organization',
            searchKey: 'organizationId',
            type: 'string' as const,
          },
        ]
      : []),
  ]
  const {
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 20 : 100 },
    globalFilter: { enabled: false },
    columnFilters:
      effectiveLogCategory === 'organization' ? [] : commonColumnFilters,
  })

  const { data, isLoading, isFetching, isError, refetch } = useQuery({
    queryKey: [
      'logs',
      effectiveLogCategory,
      isAdmin,
      pagination.pageIndex + 1,
      pagination.pageSize,
      columnFilters,
      searchParams,
      t,
    ],
    queryFn: async () => {
      const result = await fetchLogsByCategory({
        logCategory: effectiveLogCategory,
        isAdmin,
        page: pagination.pageIndex + 1,
        pageSize: pagination.pageSize,
        searchParams,
        columnFilters,
      })

      if (!result?.success) {
        throw new Error(result?.message || t('Failed to load logs'))
      }

      return result.data || DEFAULT_LOGS_DATA
    },
    placeholderData: (previousData, previousQuery) => {
      if (previousQuery?.queryKey[1] === effectiveLogCategory) {
        return previousData
      }
      return undefined
    },
  })

  const logs = data?.items || []
  const columns = useColumnsByCategory(effectiveLogCategory, isAdmin)
  const isLoadingData = isLoading || (isFetching && !data)

  const { table } = useDataTable({
    data: logs as Record<string, unknown>[],
    columns: columns as ColumnDef<Record<string, unknown>>[],
    columnFilters,
    columnVisibilityStorageKey: getColumnVisibilityStorageKey(
      effectiveLogCategory,
      isAdmin
    ),
    pagination,
    enableRowSelection: false,
    onPaginationChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total || 0,
    ensurePageInRange,
  })

  const isCommon = effectiveLogCategory === 'common'
  const isOrganization = effectiveLogCategory === 'organization'
  const emptyTitle = isOrganization
    ? t('No Organization Logs Found')
    : t('No Logs Found')
  const emptyDescription = isOrganization
    ? t(
        'Organization operations such as top-ups and quota transfers will appear here.'
      )
    : t(
        'No usage logs available. Logs will appear here once API calls are made.'
      )

  let toolbar: ReactNode = null
  if (logCategory === 'drawing' || logCategory === 'task') {
    toolbar = <TaskLogsFilterBar table={table} logCategory={logCategory} />
  }
  if (logCategory === 'common') {
    toolbar = <CommonLogsFilterBar table={table} />
  }

  if (isError) {
    return (
      <div className='flex h-full min-h-0 flex-col gap-3'>
        {toolbar}
        <ErrorState
          title={t('Failed to load logs')}
          description={t('Please try again later.')}
          onRetry={() => void refetch()}
          className='min-h-0 flex-1'
        />
      </div>
    )
  }

  return (
    <DataTablePage
      table={table}
      columns={columns as ColumnDef<Record<string, unknown>>[]}
      isLoading={isLoadingData}
      isFetching={isFetching}
      emptyTitle={emptyTitle}
      emptyDescription={emptyDescription}
      skeletonKeyPrefix='usage-log-skeleton'
      applyHeaderSize
      tableClassName={cn(
        '[&_[data-slot=table]]:text-[13px] [&_[data-slot=table]_td]:text-[13px] [&_[data-slot=table]_td_*]:text-[13px] [&_[data-slot=table]_th]:text-[13px] [&_[data-slot=table]_th_*]:text-[13px]'
      )}
      mobile={
        <UsageLogsMobileList
          table={table}
          isLoading={isLoadingData}
          logCategory={effectiveLogCategory}
          emptyTitle={emptyTitle}
          emptyDescription={emptyDescription}
        />
      }
      toolbar={toolbar}
      renderRow={(row) => {
        const logType = (row.original as Record<string, unknown>).type as
          | number
          | undefined
        let tintClass =
          isCommon && logType != null ? (logTypeRowTint[logType] ?? '') : ''
        if (isCommon && isAdmin) {
          const other = parseLogOther(
            ((row.original as Record<string, unknown>).other as string) ?? ''
          )
          if (other?.admin_info?.quota_saturation) {
            tintClass = quotaSaturationRowTint
          }
        }

        return (
          <DataTableRow
            key={row.id}
            row={row}
            className={cn('transition-colors', tintClass)}
            getColumnClassName={() => (isCommon ? 'py-2' : 'py-3.5')}
          />
        )
      }}
    />
  )
}
