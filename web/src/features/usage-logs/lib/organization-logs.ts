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

import { formatLogQuota } from '@/lib/format'

import type { OrganizationUsageLog } from '../types'

function metadataNumber(
  metadata: Record<string, unknown> | null,
  key: string
): number | null {
  const value = metadata?.[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

function metadataString(
  metadata: Record<string, unknown> | null,
  key: string
): string {
  const value = metadata?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function formatRole(role: string, t: TFunction): string {
  switch (role) {
    case 'owner':
      return t('Owner')
    case 'admin':
      return t('Admin')
    case 'member':
      return t('Member')
    default:
      return role
  }
}

function formatState(state: unknown, t: TFunction): string {
  if (state === true) return t('Enabled')
  if (state === false) return t('Disabled')
  if (state === 'active') return t('Active')
  if (state === 'disabled') return t('Disabled')
  if (state === 'dissolving') return t('Dissolving')
  if (state === 'dissolved') return t('Dissolved')
  return state == null ? '-' : String(state)
}

export function getOrganizationLogActionLabel(
  action: string,
  t: TFunction
): string {
  switch (action) {
    case 'organization.create':
      return t('Organization created')
    case 'organization.status.update':
      return t('Organization status updated')
    case 'organization.ownership.transfer':
      return t('Organization ownership transferred')
    case 'organization.member.provision':
      return t('Organization member created')
    case 'organization.member.join':
      return t('Member joined organization')
    case 'organization.member.role.update':
      return t('Organization member role updated')
    case 'organization.member.status.update':
      return t('Organization member status updated')
    case 'organization.member.limit.update':
      return t('Member quota limit updated')
    case 'organization.member.tokens.disable':
      return t('Member API keys disabled')
    case 'organization.invite.create':
      return t('Organization invite created')
    case 'organization.invite.disable':
      return t('Organization invite disabled')
    case 'organization.topup_policy.update':
      return t('Member top-up policy updated')
    case 'organization.fund.credit':
      return t('Organization wallet topped up')
    case 'organization.quota.allocate':
      return t('Member quota allocated')
    case 'organization.quota.recover':
      return t('Member quota recovered')
    default:
      return action
  }
}

export function getOrganizationLogDetails(
  log: OrganizationUsageLog,
  t: TFunction
): string {
  const metadata = log.metadata

  switch (log.action) {
    case 'organization.create':
      return metadataString(metadata, 'name')
    case 'organization.status.update':
    case 'organization.member.status.update':
      return `${formatState(metadata?.from, t)} -> ${formatState(metadata?.to, t)}`
    case 'organization.member.role.update': {
      const from = metadataString(metadata, 'from_role')
      const to = metadataString(metadata, 'to_role')
      return from && to ? `${formatRole(from, t)} -> ${formatRole(to, t)}` : ''
    }
    case 'organization.member.provision':
    case 'organization.member.join': {
      const role =
        metadataString(metadata, 'organization_role') ||
        metadataString(metadata, 'role')
      return role ? formatRole(role, t) : ''
    }
    case 'organization.member.limit.update': {
      const from = metadataNumber(metadata, 'from')
      const to = metadataNumber(metadata, 'to')
      const formatLimit = (value: number | null) =>
        value == null ? t('Unlimited') : formatLogQuota(value)
      return `${formatLimit(from)} -> ${formatLimit(to)}`
    }
    case 'organization.member.tokens.disable': {
      const count = metadataNumber(metadata, 'disabled_token_count')
      return count == null ? '' : t('{{count}} API keys disabled', { count })
    }
    case 'organization.invite.create': {
      const prefix = metadataString(metadata, 'code_prefix')
      const maxUses = metadataNumber(metadata, 'max_uses')
      const parts = [prefix ? `${t('Invite code')}: ${prefix}...` : '']
      if (maxUses != null) {
        parts.push(
          maxUses === 0
            ? t('Unlimited uses')
            : t('{{count}} uses', { count: maxUses })
        )
      }
      return parts.filter(Boolean).join(' · ')
    }
    case 'organization.invite.disable': {
      const prefix = metadataString(metadata, 'code_prefix')
      return prefix ? `${t('Invite code')}: ${prefix}...` : ''
    }
    case 'organization.topup_policy.update':
      return formatState(metadata?.allow_member_topup, t)
    case 'organization.fund.credit': {
      const amount =
        metadataNumber(metadata, 'amount') ??
        metadataNumber(metadata, 'pool_quota_delta')
      const balance = metadataNumber(metadata, 'pool_quota_after')
      const parts = []
      if (amount != null) {
        parts.push(`${t('Amount')}: ${formatLogQuota(Math.abs(amount))}`)
      }
      if (balance != null) {
        parts.push(`${t('Organization balance')}: ${formatLogQuota(balance)}`)
      }
      return parts.join(' · ')
    }
    case 'organization.quota.allocate':
    case 'organization.quota.recover': {
      const amount = metadataNumber(metadata, 'user_quota_delta')
      const balance = metadataNumber(metadata, 'user_quota_after')
      const parts = []
      if (amount != null) {
        parts.push(`${t('Amount')}: ${formatLogQuota(Math.abs(amount))}`)
      }
      if (balance != null) {
        parts.push(`${t('Member balance')}: ${formatLogQuota(balance)}`)
      }
      return parts.join(' · ')
    }
    default:
      return ''
  }
}

export function getOrganizationLogActorLabel(
  username: string,
  userID: number | undefined,
  showIDs: boolean,
  t: TFunction
): string {
  const name = username.trim()
  if (!name && (!userID || userID <= 0)) return t('System')
  const label = name || t('User')
  return showIDs && userID && userID > 0 ? `${label} (#${userID})` : label
}

export function getOrganizationLogTargetLabel(
  log: OrganizationUsageLog,
  showIDs: boolean,
  t: TFunction
): string {
  let fallback = t('Target')
  if (log.target_type === 'user') fallback = t('Member')
  if (log.target_type === 'organization') fallback = t('Organization')
  if (log.target_type === 'organization_invite') {
    fallback = t('Organization invite')
  }
  const name = log.target_name || fallback
  return showIDs && log.target_id ? `${name} (#${log.target_id})` : name
}
