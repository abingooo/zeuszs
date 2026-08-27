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
import { afterEach, describe, expect, test } from 'vitest'

import { api } from '@/lib/api'

import { createOrganization, provisionOrganizationMember } from '../api'
import {
  getProvisionOrganizationMemberSchema,
  ORGANIZATION_MEMBER_ROLE_OPTIONS,
  PROVISION_ORGANIZATION_MEMBER_DEFAULTS,
} from '../lib/organization-member-form'
import type {
  CreateOrganizationInput,
  ProvisionOrganizationMemberInput,
} from '../types'

type ApiPost = (
  url: string,
  data: unknown
) => Promise<{ data: Record<string, unknown> }>

const apiClient = api as unknown as { post: ApiPost }
const originalPost = apiClient.post

afterEach(() => {
  apiClient.post = originalPost
})

describe('platform organization provisioning contract', () => {
  test('creates an organization with the exact new Owner account fields', async () => {
    const calls: Array<{ url: string; data: unknown }> = []
    apiClient.post = async (url, data) => {
      calls.push({ url, data })
      return {
        data: {
          success: true,
          data: { organization: {}, owner: {} },
        },
      }
    }
    const input: CreateOrganizationInput = {
      name: 'Research Lab',
      owner_username: 'lab-owner',
      owner_password: 'password123',
      owner_display_name: 'Lab Owner',
      owner_email: 'owner@example.com',
      allow_member_topup: false,
    }

    await createOrganization(input)

    expect(calls).toEqual([
      {
        url: '/api/organization/admin/',
        data: input,
      },
    ])
  })

  test('defaults new organization accounts to admin and rejects Owner', async () => {
    expect(PROVISION_ORGANIZATION_MEMBER_DEFAULTS.organization_role).toBe(
      'admin'
    )
    expect(ORGANIZATION_MEMBER_ROLE_OPTIONS).toEqual(['admin', 'member'])
    expect(
      getProvisionOrganizationMemberSchema((key) => key).safeParse({
        ...PROVISION_ORGANIZATION_MEMBER_DEFAULTS,
        username: 'member-admin',
        password: 'password123',
        organization_role: 'owner',
      }).success
    ).toBe(false)

    const calls: Array<{ url: string; data: unknown }> = []
    apiClient.post = async (url, data) => {
      calls.push({ url, data })
      return {
        data: {
          success: true,
          data: { username: 'member-admin' },
        },
      }
    }
    const input: ProvisionOrganizationMemberInput = {
      username: 'member-admin',
      password: 'password123',
      display_name: 'Member Admin',
      email: 'member-admin@example.com',
      organization_role: 'admin',
    }

    await provisionOrganizationMember(42, input)

    expect(calls).toEqual([
      {
        url: '/api/organization/admin/42/members',
        data: input,
      },
    ])
  })
})
