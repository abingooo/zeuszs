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
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import type { Table } from '@tanstack/react-table'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { buildSearchParams } from '../lib/filter'
import { getOrganizationLogActionLabel } from '../lib/organization-logs'
import { getDefaultTimeRange } from '../lib/utils'
import type { OrganizationLogFilters } from '../types'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from './logs-filter-toolbar'
import { useLogsViewScope } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')
const ALL_ACTIONS = 'all'
const ALL_TARGET_TYPES = 'all'

const ORGANIZATION_ACTIONS = [
  'organization.create',
  'organization.status.update',
  'organization.ownership.transfer',
  'organization.member.provision',
  'organization.member.join',
  'organization.member.role.update',
  'organization.member.status.update',
  'organization.member.limit.update',
  'organization.member.tokens.disable',
  'organization.invite.create',
  'organization.invite.disable',
  'organization.topup_policy.update',
  'organization.fund.credit',
  'organization.quota.allocate',
  'organization.quota.recover',
] as const

type OrganizationLogDraft = {
  sourceKey: string
  filters: OrganizationLogFilters
}

function buildSourceKey(values: OrganizationLogFilters): string {
  return [
    values.startTime?.getTime(),
    values.endTime?.getTime(),
    values.action,
    values.organizationId,
    values.actorUserId,
    values.targetType,
    values.targetId,
    values.requestId,
  ]
    .map((value) => String(value ?? ''))
    .join('\u001f')
}

interface OrganizationLogsFilterBarProps<TData> {
  table: Table<TData>
}

export function OrganizationLogsFilterBar<TData>(
  props: OrganizationLogsFilterBarProps<TData>
) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const { canManageScope: isPlatformAdmin } = useLogsViewScope()
  const fetchingLogs = useIsFetching({ queryKey: ['logs', 'organization'] })

  const searchState = useMemo<OrganizationLogDraft>(() => {
    const { start, end } = getDefaultTimeRange()
    const filters: OrganizationLogFilters = {
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : start,
      endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
      action: searchParams.action || undefined,
      organizationId: searchParams.organizationId || undefined,
      actorUserId: searchParams.actorUserId || undefined,
      targetType: searchParams.targetType || undefined,
      targetId: searchParams.targetId || undefined,
      requestId: searchParams.requestId || undefined,
    }
    return { sourceKey: buildSourceKey(filters), filters }
  }, [
    searchParams.action,
    searchParams.actorUserId,
    searchParams.endTime,
    searchParams.organizationId,
    searchParams.requestId,
    searchParams.startTime,
    searchParams.targetId,
    searchParams.targetType,
  ])
  const [draft, setDraft] = useState<OrganizationLogDraft>(() => searchState)
  const activeDraft =
    draft.sourceKey === searchState.sourceKey ? draft : searchState
  const filters = activeDraft.filters

  const handleChange = useCallback(
    (field: keyof OrganizationLogFilters, value: Date | string | undefined) => {
      setDraft((current) => {
        const base =
          current.sourceKey === searchState.sourceKey ? current : searchState
        return {
          sourceKey: searchState.sourceKey,
          filters: { ...base.filters, [field]: value },
        }
      })
    },
    [searchState]
  )

  const handleApply = useCallback(() => {
    const filterParams = buildSearchParams(filters, 'organization')
    void navigate({
      to: '/usage-logs/$section',
      params: { section: 'organization' },
      search: { ...filterParams, page: 1 },
    })
    void queryClient.invalidateQueries({ queryKey: ['logs', 'organization'] })
  }, [filters, navigate, queryClient])

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    const resetFilters: OrganizationLogFilters = {
      startTime: start,
      endTime: end,
    }
    setDraft({
      sourceKey: buildSourceKey(resetFilters),
      filters: resetFilters,
    })
    void navigate({
      to: '/usage-logs/$section',
      params: { section: 'organization' },
      search: {
        page: 1,
        startTime: start.getTime(),
        endTime: end.getTime(),
      },
    })
    void queryClient.invalidateQueries({ queryKey: ['logs', 'organization'] })
  }, [navigate, queryClient])

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter') handleApply()
    },
    [handleApply]
  )

  const actionItems = useMemo(
    () => [
      { value: ALL_ACTIONS, label: t('All Actions') },
      ...ORGANIZATION_ACTIONS.map((action) => ({
        value: action,
        label: getOrganizationLogActionLabel(action, t),
      })),
    ],
    [t]
  )
  const selectedAction = filters.action || ALL_ACTIONS
  const selectedActionLabel =
    actionItems.find((item) => item.value === selectedAction)?.label ||
    selectedAction
  const targetTypeItems = useMemo(
    () => [
      { value: ALL_TARGET_TYPES, label: t('All Targets') },
      { value: 'user', label: t('Member') },
      { value: 'organization', label: t('Organization') },
      { value: 'organization_invite', label: t('Organization invite') },
    ],
    [t]
  )
  const selectedTargetType = filters.targetType || ALL_TARGET_TYPES
  const selectedTargetTypeLabel =
    targetTypeItems.find((item) => item.value === selectedTargetType)?.label ||
    t('All Targets')

  const dateFilter = (
    <LogsFilterField wide>
      <CompactDateTimeRangePicker
        start={filters.startTime}
        end={filters.endTime}
        onChange={({ start, end }) => {
          handleChange('startTime', start)
          handleChange('endTime', end)
        }}
      />
    </LogsFilterField>
  )
  const actionFilter = (
    <LogsFilterField>
      <Select
        items={actionItems}
        value={selectedAction}
        onValueChange={(value) =>
          handleChange(
            'action',
            value && value !== ALL_ACTIONS ? value : undefined
          )
        }
      >
        <SelectTrigger aria-label={t('Action')}>
          <SelectValue>{selectedActionLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {actionItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </LogsFilterField>
  )
  const requestFilter = (
    <LogsFilterField>
      <LogsFilterInput
        placeholder={t('Request ID')}
        value={filters.requestId || ''}
        onChange={(event) => handleChange('requestId', event.target.value)}
        onKeyDown={handleKeyDown}
      />
    </LogsFilterField>
  )
  const adminFilters = isPlatformAdmin ? (
    <>
      <LogsFilterField>
        <LogsFilterInput
          inputMode='numeric'
          placeholder={t('Organization ID')}
          value={filters.organizationId || ''}
          onChange={(event) =>
            handleChange('organizationId', event.target.value)
          }
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
      <LogsFilterField>
        <LogsFilterInput
          inputMode='numeric'
          placeholder={t('Operator User ID')}
          value={filters.actorUserId || ''}
          onChange={(event) => handleChange('actorUserId', event.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
      <LogsFilterField>
        <Select
          items={targetTypeItems}
          value={selectedTargetType}
          onValueChange={(value) =>
            handleChange(
              'targetType',
              value && value !== ALL_TARGET_TYPES ? value : undefined
            )
          }
        >
          <SelectTrigger aria-label={t('Target Type')}>
            <SelectValue>{selectedTargetTypeLabel}</SelectValue>
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {targetTypeItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </LogsFilterField>
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Target ID')}
          value={filters.targetId || ''}
          onChange={(event) => handleChange('targetId', event.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
    </>
  ) : null

  const expandedCount = [
    filters.requestId,
    isPlatformAdmin ? filters.organizationId : undefined,
    isPlatformAdmin ? filters.actorUserId : undefined,
    isPlatformAdmin ? filters.targetType : undefined,
    isPlatformAdmin ? filters.targetId : undefined,
  ].filter(Boolean).length
  const hasActiveFilters = Boolean(filters.action || expandedCount > 0)

  return (
    <LogsFilterToolbar
      table={props.table}
      primaryFilters={
        <>
          {dateFilter}
          {actionFilter}
        </>
      }
      advancedFilters={
        <>
          {requestFilter}
          {adminFilters}
        </>
      }
      mobilePinnedFilters={
        <>
          {dateFilter}
          {actionFilter}
        </>
      }
      mobileFilters={
        <>
          {requestFilter}
          {adminFilters}
        </>
      }
      hasActiveFilters={hasActiveFilters}
      hasAdvancedActiveFilters={expandedCount > 0}
      advancedFilterCount={expandedCount}
      mobileFilterCount={(filters.action ? 1 : 0) + expandedCount}
      searchLoading={fetchingLogs > 0}
      onReset={handleReset}
      onSearch={handleApply}
    />
  )
}
