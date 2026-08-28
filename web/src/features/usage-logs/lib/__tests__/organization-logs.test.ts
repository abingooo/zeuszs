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
import type { TFunction } from 'i18next'
import { describe, expect, test } from 'vitest'

import { ROLE } from '@/lib/roles'

import type { OrganizationUsageLog } from '../../types'
import { canAccessOrganizationLogs } from '../organization-log-access'
import {
  getOrganizationLogActionLabel,
  getOrganizationLogActorLabel,
  getOrganizationLogDetails,
  getOrganizationLogTargetLabel,
} from '../organization-logs'
import { buildOrganizationApiParams } from '../utils'

const t = ((key: string) => key) as unknown as TFunction

const organizationLog: OrganizationUsageLog = {
  id: 1,
  organization_id: 17,
  organization_name: 'Alpha Lab',
  actor_user_id: 42,
  actor_username: 'alice',
  action: 'organization.quota.allocate',
  target_type: 'user',
  target_id: '88',
  target_name: 'bob',
  request_id: 'request-1',
  metadata: { user_quota_delta: 1000 },
  created_at: 100,
}

describe('organization usage logs', () => {
  test('builds a concise query and converts millisecond dates to seconds', () => {
    expect(
      buildOrganizationApiParams({
        page: 2,
        pageSize: 50,
        searchParams: {
          action: 'organization.quota.allocate',
          organizationId: '17',
          actorUserId: '42',
          targetType: 'user',
          targetId: '88',
          requestId: 'request-1',
          startTime: 10_000,
          endTime: 20_000,
        },
      })
    ).toEqual({
      p: 2,
      page_size: 50,
      start_timestamp: 10,
      end_timestamp: 20,
    })
  })

  test('does not apply legacy detailed filters to the simplified view', () => {
    const params = buildOrganizationApiParams({
      page: 1,
      pageSize: 20,
      searchParams: {
        action: 'organization.quota.allocate',
        organizationId: '17',
        actorUserId: '42',
        targetType: 'user',
        targetId: '88',
        requestId: 'request-1',
        startTime: 10_000,
        endTime: 20_000,
      },
    })

    expect(params).not.toHaveProperty('organization_id')
    expect(params).not.toHaveProperty('actor_user_id')
    expect(params).not.toHaveProperty('target_type')
    expect(params).not.toHaveProperty('target_id')
    expect(params).not.toHaveProperty('request_id')
  })

  test('renders localized action labels and hides entity IDs independently of names', () => {
    expect(getOrganizationLogActionLabel(organizationLog.action, t)).toBe(
      'Member quota allocated'
    )
    expect(getOrganizationLogActorLabel('alice', 42, false, t)).toBe('alice')
    expect(getOrganizationLogTargetLabel(organizationLog, false, t)).toBe('bob')
    expect(getOrganizationLogActorLabel('alice', 42, true, t)).toBe(
      'alice (#42)'
    )
    expect(getOrganizationLogTargetLabel(organizationLog, true, t)).toBe(
      'bob (#88)'
    )
  })

  test('keeps names readable when the server omits hidden entity IDs', () => {
    const hiddenIDsLog: OrganizationUsageLog = {
      ...organizationLog,
      organization_id: undefined,
      actor_user_id: undefined,
      initiator_user_id: undefined,
      target_id: undefined,
    }

    expect(
      getOrganizationLogActorLabel(
        hiddenIDsLog.actor_username,
        hiddenIDsLog.actor_user_id,
        true,
        t
      )
    ).toBe('alice')
    expect(getOrganizationLogTargetLabel(hiddenIDsLog, true, t)).toBe('bob')
    expect(getOrganizationLogActorLabel('', undefined, false, t)).toBe('System')
  })

  test('renders a member-safe organization top-up amount', () => {
    expect(
      getOrganizationLogDetails(
        {
          ...organizationLog,
          action: 'organization.fund.credit',
          metadata: { amount: 500_000 },
        },
        t
      )
    ).toContain('Amount:')
  })

  test.each([
    {
      name: 'platform admin',
      user: { id: 1, username: 'admin', role: ROLE.ADMIN },
      expected: true,
    },
    {
      name: 'active tenant member',
      user: {
        id: 2,
        username: 'member',
        role: ROLE.USER,
        organization_id: 17,
        organization_status: 'active' as const,
        organization_is_default: false,
      },
      expected: true,
    },
    {
      name: 'default organization member',
      user: {
        id: 3,
        username: 'default-member',
        role: ROLE.USER,
        organization_id: 1,
        organization_status: 'active' as const,
        organization_is_default: true,
      },
      expected: false,
    },
    {
      name: 'inactive tenant member',
      user: {
        id: 4,
        username: 'inactive-member',
        role: ROLE.USER,
        organization_id: 17,
        organization_status: 'disabled' as const,
        organization_is_default: false,
      },
      expected: false,
    },
  ])('applies organization log access for $name', ({ user, expected }) => {
    expect(canAccessOrganizationLogs(user)).toBe(expected)
  })
})
