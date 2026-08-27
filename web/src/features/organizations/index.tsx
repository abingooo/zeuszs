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
import { Add01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

import { CreateOrganizationDialog } from './components/create-organization-dialog'
import { OrganizationDetailsSheet } from './components/organization-details-sheet'
import { OrganizationsList } from './components/organizations-list'
import type { Organization } from './types'

export function Organizations() {
  const { t } = useTranslation()
  const [createOpen, setCreateOpen] = useState(false)
  const [selectedOrganization, setSelectedOrganization] =
    useState<Organization | null>(null)

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Organizations')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button type='button' onClick={() => setCreateOpen(true)}>
            <HugeiconsIcon
              icon={Add01Icon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('Create organization')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <OrganizationsList
            onCreate={() => setCreateOpen(true)}
            onManage={setSelectedOrganization}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <CreateOrganizationDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
      />
      <OrganizationDetailsSheet
        organization={selectedOrganization}
        onOpenChange={(open) => {
          if (!open) setSelectedOrganization(null)
        }}
        onOrganizationChange={setSelectedOrganization}
      />
    </>
  )
}
