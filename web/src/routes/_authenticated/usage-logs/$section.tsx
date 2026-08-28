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
import { createFileRoute, redirect } from '@tanstack/react-router'
import z from 'zod'

import { UsageLogs } from '@/features/usage-logs'
import {
  LOG_TYPE_ALL_VALUE,
  ORGANIZATION_LOG_TYPE_VALUE,
} from '@/features/usage-logs/constants'
import { canAccessOrganizationLogs } from '@/features/usage-logs/lib/organization-log-access'
import {
  isUsageLogsSectionId,
  USAGE_LOGS_DEFAULT_SECTION,
} from '@/features/usage-logs/section-registry'
import { useAuthStore } from '@/stores/auth-store'

const logTypeValues = [
  '0',
  '1',
  '2',
  '3',
  '4',
  '5',
  '6',
  '7',
  ORGANIZATION_LOG_TYPE_VALUE,
] as const
const logTypeSearchSchema = z
  .preprocess((value) => {
    if (value == null || value === '') return undefined
    return Array.isArray(value) ? value : [value]
  }, z.array(z.enum(logTypeValues)).max(1).optional())
  .catch([])

const usageLogsSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(undefined),
  type: logTypeSearchSchema.optional(),
  filter: z.string().optional().catch(''),
  model: z.string().optional().catch(''),
  token: z.string().optional().catch(''),
  channel: z.string().optional().catch(''),
  group: z.string().optional().catch(''),
  username: z.string().optional().catch(''),
  organizationId: z.string().optional().catch(''),
  actorUserId: z.string().optional().catch(''),
  action: z.string().optional().catch(''),
  targetType: z.string().optional().catch(''),
  targetId: z.string().optional().catch(''),
  requestId: z.string().optional().catch(''),
  upstreamRequestId: z.string().optional().catch(''),
  startTime: z.number().optional(),
  endTime: z.number().optional(),
})

export const Route = createFileRoute('/_authenticated/usage-logs/$section')({
  beforeLoad: ({ params, search }) => {
    const canAccessOrganization = canAccessOrganizationLogs(
      useAuthStore.getState().auth.user
    )
    if (params.section === 'organization') {
      throw redirect({
        to: '/usage-logs/$section',
        params: { section: USAGE_LOGS_DEFAULT_SECTION },
        search: {
          ...search,
          type: [
            canAccessOrganization
              ? ORGANIZATION_LOG_TYPE_VALUE
              : LOG_TYPE_ALL_VALUE,
          ],
        },
        replace: true,
      })
    }
    if (!isUsageLogsSectionId(params.section)) {
      throw redirect({
        to: '/usage-logs/$section',
        params: { section: USAGE_LOGS_DEFAULT_SECTION },
      })
    }
    const requestsOrganization =
      Array.isArray(search?.type) &&
      search.type.length === 1 &&
      search.type[0] === ORGANIZATION_LOG_TYPE_VALUE
    if (
      params.section === USAGE_LOGS_DEFAULT_SECTION &&
      requestsOrganization &&
      !canAccessOrganization
    ) {
      throw redirect({
        to: '/usage-logs/$section',
        params: { section: USAGE_LOGS_DEFAULT_SECTION },
        search: { ...search, type: [LOG_TYPE_ALL_VALUE] },
        replace: true,
      })
    }
    // type 仅 common 使用，非 common 时清掉 URL 里的 type
    const hasTypeSearch = Array.isArray(search?.type)
      ? search.type.length > 0
      : search?.type != null && search.type !== ''
    if (params.section !== 'common' && hasTypeSearch) {
      throw redirect({
        to: '/usage-logs/$section',
        params: { section: params.section },
        search: { ...search, type: undefined },
        replace: true,
      })
    }
  },
  validateSearch: usageLogsSearchSchema,
  component: UsageLogs,
})
