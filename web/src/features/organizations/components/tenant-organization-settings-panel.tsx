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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLegend,
  FieldSet,
  FieldTitle,
} from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import {
  tenantOrganizationKeys,
  updateTenantOrganizationTopupPolicy,
} from '../tenant-api'
import type { TenantOrganizationSummary } from '../types'

export function TenantOrganizationSettingsPanel(props: {
  summary: TenantOrganizationSummary
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const topupMutation = useMutation({
    mutationFn: updateTenantOrganizationTopupPolicy,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: tenantOrganizationKeys.summary(),
      })
      void queryClient.invalidateQueries({
        queryKey: tenantOrganizationKeys.audit(),
      })
      toast.success(t('Member top-up policy updated'))
    },
    onError: handleServerError,
  })

  return (
    <FieldSet>
      <FieldLegend>{t('Organization policy')}</FieldLegend>
      <FieldDescription>
        {t('This policy applies to ordinary members only.')}
      </FieldDescription>
      <FieldGroup>
        <Field orientation='horizontal'>
          <FieldContent>
            <FieldTitle>{t('Allow member top-ups')}</FieldTitle>
            <FieldDescription>
              {t(
                'Controls whether ordinary members may add funds to their own single balance.'
              )}
            </FieldDescription>
          </FieldContent>
          <Switch
            checked={props.summary.allow_member_topup}
            disabled={topupMutation.isPending}
            onCheckedChange={(checked) => topupMutation.mutate(checked)}
            aria-label={t('Allow member top-ups')}
          />
        </Field>
      </FieldGroup>
    </FieldSet>
  )
}
