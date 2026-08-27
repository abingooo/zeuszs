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
import { Building03Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useAuthStore } from '@/stores/auth-store'

import {
  TenantOrganizationAuditPanel,
  TenantOrganizationLedgerPanel,
} from './components/tenant-organization-history-panels'
import { TenantOrganizationInvitesPanel } from './components/tenant-organization-invites-panel'
import { TenantOrganizationMembersPanel } from './components/tenant-organization-members-panel'
import { TenantOrganizationOverview } from './components/tenant-organization-overview'
import { TenantOrganizationSettingsPanel } from './components/tenant-organization-settings-panel'
import {
  getTenantOrganizationSummary,
  tenantOrganizationKeys,
} from './tenant-api'
import type { TenantOrganizationSummary } from './types'

export function OrganizationWorkspaceView(props: {
  summary: TenantOrganizationSummary
}) {
  const { t } = useTranslation()
  const canManage =
    props.summary.current_user_role === 'owner' ||
    props.summary.current_user_role === 'admin'

  if (!canManage) {
    return <TenantOrganizationOverview summary={props.summary} />
  }

  return (
    <Tabs defaultValue='overview' className='min-h-0 flex-1 gap-0'>
      <div className='overflow-x-auto border-b'>
        <TabsList variant='line' className='h-10'>
          <TabsTrigger value='overview'>{t('Overview')}</TabsTrigger>
          <TabsTrigger value='members'>{t('Members')}</TabsTrigger>
          <TabsTrigger value='invites'>{t('Invites')}</TabsTrigger>
          <TabsTrigger value='ledger'>{t('Ledger')}</TabsTrigger>
          <TabsTrigger value='audit'>{t('Audit')}</TabsTrigger>
          <TabsTrigger value='settings'>{t('Settings')}</TabsTrigger>
        </TabsList>
      </div>
      <TabsContent value='overview' className='min-h-0 overflow-auto py-4'>
        <TenantOrganizationOverview summary={props.summary} />
      </TabsContent>
      <TabsContent value='members' className='min-h-0 overflow-auto py-4'>
        <TenantOrganizationMembersPanel summary={props.summary} />
      </TabsContent>
      <TabsContent value='invites' className='min-h-0 overflow-auto py-4'>
        <TenantOrganizationInvitesPanel
          organizationId={props.summary.organization_id}
        />
      </TabsContent>
      <TabsContent value='ledger' className='min-h-0 overflow-auto py-4'>
        <TenantOrganizationLedgerPanel />
      </TabsContent>
      <TabsContent value='audit' className='min-h-0 overflow-auto py-4'>
        <TenantOrganizationAuditPanel />
      </TabsContent>
      <TabsContent value='settings' className='min-h-0 overflow-auto py-4'>
        <TenantOrganizationSettingsPanel summary={props.summary} />
      </TabsContent>
    </Tabs>
  )
}

export function OrganizationWorkspace() {
  const { t } = useTranslation()
  const organizationRole = useAuthStore(
    (state) => state.auth.user?.organization_role
  )
  const title =
    organizationRole === 'owner' || organizationRole === 'admin'
      ? t('Manage organization')
      : t('My organization')
  const summaryQuery = useQuery({
    queryKey: tenantOrganizationKeys.summary(),
    queryFn: getTenantOrganizationSummary,
  })

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{title}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {summaryQuery.isLoading && (
          <div className='flex flex-col gap-3'>
            <Skeleton className='h-24 w-full' />
            <Skeleton className='h-40 w-full' />
          </div>
        )}
        {summaryQuery.isError && (
          <Empty className='border'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <HugeiconsIcon icon={Building03Icon} strokeWidth={2} />
              </EmptyMedia>
              <EmptyTitle>{t('Unable to load organization')}</EmptyTitle>
              <EmptyDescription>
                {t('Refresh the organization summary and try again.')}
              </EmptyDescription>
            </EmptyHeader>
            <Button
              type='button'
              variant='outline'
              onClick={() => void summaryQuery.refetch()}
            >
              {t('Retry')}
            </Button>
          </Empty>
        )}
        {summaryQuery.data && (
          <OrganizationWorkspaceView summary={summaryQuery.data} />
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
