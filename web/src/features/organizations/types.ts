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
export type OrganizationStatus =
  | 'active'
  | 'disabled'
  | 'dissolving'
  | 'dissolved'

export type OrganizationRole = 'owner' | 'admin' | 'member'
export type OrganizationMemberStatus = 'active' | 'disabled'
export type OrganizationInviteStatus = 'active' | 'disabled'

export type Organization = {
  id: number
  name: string
  system_key?: string
  status: OrganizationStatus
  owner_user_id: number
  owner_username?: string
  allow_member_topup: boolean
  policy_version: number
  member_count: number
  created_at: number
  updated_at: number
}

export type OrganizationMember = {
  user_id: number
  username: string
  display_name: string
  email: string
  platform_role: number
  organization_id: number
  organization_role: OrganizationRole
  organization_status: OrganizationMemberStatus
  quota: number
  used_quota: number
  request_count: number
  recoverable_quota: number
  consumed_quota: number
  consumption_limit?: number
  created_at: number
}

export type OrganizationInvite = {
  id: number
  organization_id: number
  code_prefix: string
  status: OrganizationInviteStatus
  max_uses: number
  used_count: number
  expires_at: number
  default_role: OrganizationRole
  created_by: number
  created_at: number
  updated_at: number
  code?: string
}

export type PageData<T> = {
  items: T[]
  total: number
  page: number
  page_size: number
}

export type ApiResponse<T> = {
  success: boolean
  message?: string
  code?: string
  data?: T
}

export type CreateOrganizationInput = {
  name: string
  owner_username: string
  owner_password: string
  owner_display_name: string
  owner_email: string
  allow_member_topup: boolean
}

export type ProvisionOrganizationMemberInput = {
  username: string
  password: string
  display_name: string
  email: string
  organization_role: Exclude<OrganizationRole, 'owner'>
}

export type ProvisionedOrganizationOwner = {
  user_id: number
  username: string
  display_name: string
  email?: string
  organization_id: number
  organization_role: OrganizationRole
  organization_status: OrganizationMemberStatus
}

export type ProvisionedOrganization = {
  organization: Organization
  owner: ProvisionedOrganizationOwner
}

export type CreateOrganizationInviteInput = {
  code?: string
  max_uses: number
  expires_at: number
}

export type CreditOrganizationFundInput = {
  amount: number
  reference?: string
}

export type CreditOrganizationFundResult = {
  ledger_id: number
  pool_quota_after: number
  already_applied: boolean
  user_quota_after: number
  recoverable_quota_after: number
}

export type TenantOrganizationSummary = {
  organization_id: number
  name: string
  status: OrganizationStatus
  current_user_role: OrganizationRole
  member_status: OrganizationMemberStatus
  allow_member_topup: boolean
  owner_user_id?: number
  policy_version?: number
  fund_quota?: number
  member_count?: number
  created_at?: number
  updated_at?: number
}

export type TenantQuotaTransferResult = {
  ledger_id: number
  user_quota_after: number
  pool_quota_after: number
  recoverable_quota_after: number
  already_applied: boolean
}

export type TenantOrganizationLedgerEntry = {
  id: number
  user_id: number
  project_id?: number
  operation: string
  source_type: string
  source_id: string
  actor_user_id: number
  request_id: string
  user_quota_delta: number
  pool_quota_delta: number
  recoverable_quota_delta: number
  user_quota_after: number
  pool_quota_after: number
  recoverable_quota_after: number
  related_ledger_id?: number
  status: string
  created_at: number
}

export type TenantOrganizationAuditEntry = {
  id: number
  actor_user_id: number
  action: string
  target_type: string
  target_id: string
  request_id: string
  metadata: unknown
  created_at: number
}

export type ListOrganizationsParams = {
  page: number
  pageSize: number
  status?: OrganizationStatus
}

export type ListOrganizationMembersParams = {
  organizationId: number
  page: number
  pageSize: number
  keyword?: string
  status?: OrganizationMemberStatus
}

export type ListOrganizationInvitesParams = {
  organizationId: number
  page: number
  pageSize: number
  status?: OrganizationInviteStatus
}
