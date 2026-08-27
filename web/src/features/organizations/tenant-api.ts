import { nanoid } from 'nanoid'

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

import { requireApiData } from './api'
import type {
  ApiResponse,
  CreateOrganizationInviteInput,
  ListOrganizationInvitesParams,
  ListOrganizationMembersParams,
  Organization,
  OrganizationInvite,
  OrganizationMember,
  OrganizationMemberStatus,
  PageData,
  TenantOrganizationAuditEntry,
  TenantOrganizationLedgerEntry,
  TenantOrganizationSummary,
  TenantQuotaTransferResult,
} from './types'

type TenantPagedParams = {
  page: number
  pageSize: number
}

export const tenantOrganizationKeys = {
  all: ['tenant-organization'] as const,
  summary: () => [...tenantOrganizationKeys.all, 'summary'] as const,
  members: () => [...tenantOrganizationKeys.all, 'members'] as const,
  memberList: (params: ListOrganizationMembersParams) =>
    [...tenantOrganizationKeys.members(), params] as const,
  invites: () => [...tenantOrganizationKeys.all, 'invites'] as const,
  inviteList: (params: ListOrganizationInvitesParams) =>
    [...tenantOrganizationKeys.invites(), params] as const,
  ledger: () => [...tenantOrganizationKeys.all, 'ledger'] as const,
  ledgerList: (params: TenantPagedParams) =>
    [...tenantOrganizationKeys.ledger(), params] as const,
  audit: () => [...tenantOrganizationKeys.all, 'audit'] as const,
  auditList: (params: TenantPagedParams) =>
    [...tenantOrganizationKeys.audit(), params] as const,
}

export async function getTenantOrganizationSummary(): Promise<TenantOrganizationSummary> {
  const response = await api.get<ApiResponse<TenantOrganizationSummary>>(
    '/api/organization/self'
  )
  return requireApiData(response.data)
}

export async function listTenantOrganizationMembers(
  params: ListOrganizationMembersParams
): Promise<PageData<OrganizationMember>> {
  const response = await api.get<ApiResponse<PageData<OrganizationMember>>>(
    '/api/organization/members',
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

export async function updateTenantMemberStatus(
  userId: number,
  status: OrganizationMemberStatus
): Promise<OrganizationMember> {
  const response = await api.patch<ApiResponse<OrganizationMember>>(
    `/api/organization/members/${userId}/status`,
    { status }
  )
  return requireApiData(response.data)
}

export async function updateTenantMemberConsumptionLimit(
  userId: number,
  consumptionLimit: number | null
): Promise<OrganizationMember> {
  const response = await api.patch<ApiResponse<OrganizationMember>>(
    `/api/organization/members/${userId}/consumption-limit`,
    { consumption_limit: consumptionLimit }
  )
  return requireApiData(response.data)
}

export async function allocateTenantMemberQuota(
  userId: number,
  amount: number
): Promise<TenantQuotaTransferResult> {
  const response = await api.post<ApiResponse<TenantQuotaTransferResult>>(
    `/api/organization/members/${userId}/allocate`,
    { amount },
    { headers: { 'Idempotency-Key': nanoid() } }
  )
  return requireApiData(response.data)
}

export async function recoverTenantMemberQuota(
  userId: number,
  amount: number
): Promise<TenantQuotaTransferResult> {
  const response = await api.post<ApiResponse<TenantQuotaTransferResult>>(
    `/api/organization/members/${userId}/recover`,
    { amount },
    { headers: { 'Idempotency-Key': nanoid() } }
  )
  return requireApiData(response.data)
}

export async function disableTenantMemberTokens(
  userId: number
): Promise<{ disabled_token_count: number }> {
  const response = await api.post<
    ApiResponse<{ disabled_token_count: number }>
  >(`/api/organization/members/${userId}/tokens/disable`)
  return requireApiData(response.data)
}

export async function listTenantOrganizationInvites(
  params: ListOrganizationInvitesParams
): Promise<PageData<OrganizationInvite>> {
  const response = await api.get<ApiResponse<PageData<OrganizationInvite>>>(
    '/api/organization/invites',
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

export async function createTenantOrganizationInvite(
  input: CreateOrganizationInviteInput
): Promise<OrganizationInvite> {
  const response = await api.post<ApiResponse<OrganizationInvite>>(
    '/api/organization/invites',
    input
  )
  return requireApiData(response.data)
}

export async function disableTenantOrganizationInvite(
  inviteId: number
): Promise<OrganizationInvite> {
  const response = await api.patch<ApiResponse<OrganizationInvite>>(
    `/api/organization/invites/${inviteId}/status`
  )
  return requireApiData(response.data)
}

export async function updateTenantOrganizationTopupPolicy(
  allowMemberTopup: boolean
): Promise<Organization> {
  const response = await api.patch<ApiResponse<Organization>>(
    '/api/organization/topup-policy',
    { allow_member_topup: allowMemberTopup }
  )
  return requireApiData(response.data)
}

export async function listTenantOrganizationLedger(
  params: TenantPagedParams
): Promise<PageData<TenantOrganizationLedgerEntry>> {
  const response = await api.get<
    ApiResponse<PageData<TenantOrganizationLedgerEntry>>
  >('/api/organization/ledger', {
    params: { p: params.page, page_size: params.pageSize },
  })
  return requireApiData(response.data)
}

export async function listTenantOrganizationAudit(
  params: TenantPagedParams
): Promise<PageData<TenantOrganizationAuditEntry>> {
  const response = await api.get<
    ApiResponse<PageData<TenantOrganizationAuditEntry>>
  >('/api/organization/audit', {
    params: { p: params.page, page_size: params.pageSize },
  })
  return requireApiData(response.data)
}
