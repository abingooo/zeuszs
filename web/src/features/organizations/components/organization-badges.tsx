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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import type {
  OrganizationInviteStatus,
  OrganizationMemberStatus,
  OrganizationRole,
  OrganizationStatus,
} from '../types'

export function OrganizationStatusBadge(props: { status: OrganizationStatus }) {
  const { t } = useTranslation()
  if (props.status === 'active') return <Badge>{t('Active')}</Badge>
  if (props.status === 'disabled') {
    return <Badge variant='secondary'>{t('Disabled')}</Badge>
  }
  if (props.status === 'dissolving') {
    return <Badge variant='warning'>{t('Dissolving')}</Badge>
  }
  return <Badge variant='outline'>{t('Dissolved')}</Badge>
}

export function OrganizationRoleBadge(props: { role: OrganizationRole }) {
  const { t } = useTranslation()
  if (props.role === 'owner') return <Badge>{t('Owner')}</Badge>
  if (props.role === 'admin') {
    return <Badge variant='secondary'>{t('Organization admin')}</Badge>
  }
  return <Badge variant='outline'>{t('Member')}</Badge>
}

export function MemberStatusBadge(props: { status: OrganizationMemberStatus }) {
  const { t } = useTranslation()
  return props.status === 'active' ? (
    <Badge>{t('Active')}</Badge>
  ) : (
    <Badge variant='secondary'>{t('Disabled')}</Badge>
  )
}

export function InviteStatusBadge(props: { status: OrganizationInviteStatus }) {
  const { t } = useTranslation()
  return props.status === 'active' ? (
    <Badge>{t('Active')}</Badge>
  ) : (
    <Badge variant='secondary'>{t('Disabled')}</Badge>
  )
}
