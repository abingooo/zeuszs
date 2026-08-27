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
import { api } from '@/lib/api'

import type {
  ApiResponse,
  CreateOrganizationInput,
  CreateOrganizationInviteInput,
  CreditOrganizationFundInput,
  CreditOrganizationFundResult,
  ListOrganizationInvitesParams,
  ListOrganizationMembersParams,
  ListOrganizationsParams,
  Organization,
  OrganizationInvite,
  OrganizationMember,
  OrganizationRole,
  OrganizationStatus,
  PageData,
  ProvisionOrganizationMemberInput,
  ProvisionedOrganization,
} from './types'

export function requireApiData<T>(response: ApiResponse<T>): T {
  if (!response.success || response.data === undefined) {
    throw new Error(response.message || 'Organization operation failed')
  }
  return response.data
}

export const organizationKeys = {
  all: ['organizations'] as const,
  lists: () => [...organizationKeys.all, 'list'] as const,
  list: (params: ListOrganizationsParams) =>
    [...organizationKeys.lists(), params] as const,
  members: (organizationId: number) =>
    [...organizationKeys.all, organizationId, 'members'] as const,
  memberList: (params: ListOrganizationMembersParams) =>
    [...organizationKeys.members(params.organizationId), params] as const,
  invites: (organizationId: number) =>
    [...organizationKeys.all, organizationId, 'invites'] as const,
  inviteList: (params: ListOrganizationInvitesParams) =>
    [...organizationKeys.invites(params.organizationId), params] as const,
}

export async function listOrganizations(
  params: ListOrganizationsParams
): Promise<PageData<Organization>> {
  const response = await api.get<ApiResponse<PageData<Organization>>>(
    '/api/organization/admin/',
    {
      params: {
        p: params.page,
        page_size: params.pageSize,
        status: params.status,
      },
    }
  )
  return requireApiData(response.data)
}

export async function createOrganization(
  input: CreateOrganizationInput
): Promise<ProvisionedOrganization> {
  const response = await api.post<ApiResponse<ProvisionedOrganization>>(
    '/api/organization/admin/',
    input
  )
  return requireApiData(response.data)
}

export async function updateOrganizationStatus(
  organizationId: number,
  status: OrganizationStatus
): Promise<Organization> {
  const response = await api.patch<ApiResponse<Organization>>(
    `/api/organization/admin/${organizationId}/status`,
    { status }
  )
  return requireApiData(response.data)
}

export async function listOrganizationMembers(
  params: ListOrganizationMembersParams
): Promise<PageData<OrganizationMember>> {
  const response = await api.get<ApiResponse<PageData<OrganizationMember>>>(
    `/api/organization/admin/${params.organizationId}/members`,
    {
      params: {
        p: params.page,
        page_size: params.pageSize,
        keyword: params.keyword,
        status: params.status,
      },
    }
  )
  return requireApiData(response.data)
}

export async function provisionOrganizationMember(
  organizationId: number,
  input: ProvisionOrganizationMemberInput
): Promise<OrganizationMember> {
  const response = await api.post<ApiResponse<OrganizationMember>>(
    `/api/organization/admin/${organizationId}/members`,
    input
  )
  return requireApiData(response.data)
}

export async function updateOrganizationMemberRole(
  organizationId: number,
  userId: number,
  role: Exclude<OrganizationRole, 'owner'>
): Promise<OrganizationMember> {
  const response = await api.patch<ApiResponse<OrganizationMember>>(
    `/api/organization/admin/${organizationId}/members/${userId}/role`,
    { role }
  )
  return requireApiData(response.data)
}

export async function transferOrganizationOwnership(
  organizationId: number,
  newOwnerUserId: number
): Promise<Organization> {
  const response = await api.patch<ApiResponse<Organization>>(
    `/api/organization/admin/${organizationId}/ownership`,
    { new_owner_user_id: newOwnerUserId }
  )
  return requireApiData(response.data)
}

export async function updateOrganizationTopupPolicy(
  organizationId: number,
  allowMemberTopup: boolean
): Promise<Organization> {
  const response = await api.patch<ApiResponse<Organization>>(
    `/api/organization/admin/${organizationId}/topup-policy`,
    { allow_member_topup: allowMemberTopup }
  )
  return requireApiData(response.data)
}

export async function creditOrganizationFund(
  organizationId: number,
  input: CreditOrganizationFundInput
): Promise<CreditOrganizationFundResult> {
  const response = await api.post<ApiResponse<CreditOrganizationFundResult>>(
    `/api/organization/admin/${organizationId}/fund-credit`,
    input
  )
  return requireApiData(response.data)
}

export async function listOrganizationInvites(
  params: ListOrganizationInvitesParams
): Promise<PageData<OrganizationInvite>> {
  const response = await api.get<ApiResponse<PageData<OrganizationInvite>>>(
    `/api/organization/admin/${params.organizationId}/invites`,
    {
      params: {
        p: params.page,
        page_size: params.pageSize,
        status: params.status,
      },
    }
  )
  return requireApiData(response.data)
}

export async function createOrganizationInvite(
  organizationId: number,
  input: CreateOrganizationInviteInput
): Promise<OrganizationInvite> {
  const response = await api.post<ApiResponse<OrganizationInvite>>(
    `/api/organization/admin/${organizationId}/invites`,
    input
  )
  return requireApiData(response.data)
}

export async function disableOrganizationInvite(
  organizationId: number,
  inviteId: number
): Promise<OrganizationInvite> {
  const response = await api.patch<ApiResponse<OrganizationInvite>>(
    `/api/organization/admin/${organizationId}/invites/${inviteId}/status`
  )
  return requireApiData(response.data)
}
