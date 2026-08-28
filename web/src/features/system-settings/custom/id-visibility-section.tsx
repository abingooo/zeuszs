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
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const OPTION_KEY = 'custom_setting.id_visibility_enabled' as const

const idVisibilitySchema = z.object({
  idVisibilityEnabled: z.boolean(),
})

type IDVisibilityFormValues = z.infer<typeof idVisibilitySchema>

export function IDVisibilitySection(props: {
  defaultValue: IDVisibilityFormValues['idVisibilityEnabled']
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<IDVisibilityFormValues>({
    resolver: zodResolver(idVisibilitySchema),
    defaultValues: { idVisibilityEnabled: props.defaultValue },
  })

  useResetForm(form, { idVisibilityEnabled: props.defaultValue })

  const onSubmit = async (values: IDVisibilityFormValues) => {
    const result = await updateOption.mutateAsync({
      key: OPTION_KEY,
      value: values.idVisibilityEnabled,
    })
    if (result.success) {
      form.reset(values)
    }
  }

  return (
    <SettingsSection title={t('ID Visibility')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || form.formState.isSubmitting}
            isSaveDisabled={!form.formState.isDirty}
          />
          <FormField
            control={form.control}
            name='idVisibilityEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('ID Visibility')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Allow non-platform users to view organization and user IDs. Platform administrators and platform owners can always view them.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={
                      updateOption.isPending || form.formState.isSubmitting
                    }
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
