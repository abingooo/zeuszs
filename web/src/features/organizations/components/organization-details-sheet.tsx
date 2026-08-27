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

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { Organization } from '../types'
import { OrganizationStatusBadge } from './organization-badges'
import { OrganizationInvitesPanel } from './organization-invites-panel'
import { OrganizationMembersPanel } from './organization-members-panel'
import { OrganizationSettingsPanel } from './organization-settings-panel'

type OrganizationDetailsSheetProps = {
  organization: Organization | null
  onOpenChange: (open: boolean) => void
  onOrganizationChange: (organization: Organization) => void
}

export function OrganizationDetailsSheet(props: OrganizationDetailsSheetProps) {
  const { t } = useTranslation()
  const organization = props.organization

  return (
    <Sheet open={organization !== null} onOpenChange={props.onOpenChange}>
      <SheetContent className='w-full sm:max-w-3xl'>
        {organization && (
          <>
            <SheetHeader className='border-b pr-12'>
              <div className='flex min-w-0 items-center gap-2'>
                <SheetTitle className='truncate'>
                  {organization.name}
                </SheetTitle>
                <OrganizationStatusBadge status={organization.status} />
              </div>
              <SheetDescription>
                {t('Organization ID: {{id}}', { id: organization.id })}
              </SheetDescription>
            </SheetHeader>

            <Tabs
              key={organization.id}
              defaultValue='members'
              className='min-h-0 flex-1 gap-0'
            >
              <div className='overflow-x-auto border-b px-4'>
                <TabsList variant='line' className='h-10'>
                  <TabsTrigger value='members'>{t('Members')}</TabsTrigger>
                  <TabsTrigger value='invites'>{t('Invites')}</TabsTrigger>
                  <TabsTrigger value='settings'>{t('Settings')}</TabsTrigger>
                </TabsList>
              </div>
              <TabsContent
                value='members'
                className='min-h-0 overflow-auto p-4'
              >
                <OrganizationMembersPanel
                  organization={organization}
                  onOrganizationChange={props.onOrganizationChange}
                />
              </TabsContent>
              <TabsContent
                value='invites'
                className='min-h-0 overflow-auto p-4'
              >
                <OrganizationInvitesPanel organizationId={organization.id} />
              </TabsContent>
              <TabsContent
                value='settings'
                className='min-h-0 overflow-auto p-4'
              >
                <OrganizationSettingsPanel
                  organization={organization}
                  onOrganizationChange={props.onOrganizationChange}
                />
              </TabsContent>
            </Tabs>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}
