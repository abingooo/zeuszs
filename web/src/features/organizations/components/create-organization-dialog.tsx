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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { FieldGroup } from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import { createOrganization, organizationKeys } from '../api'
import {
  CREATE_ORGANIZATION_DEFAULTS,
  getCreateOrganizationSchema,
  type CreateOrganizationFormValues,
} from '../lib/organization-form'

type CreateOrganizationDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CreateOrganizationDialog(props: CreateOrganizationDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const schema = useMemo(() => getCreateOrganizationSchema(t), [t])
  const form = useForm<CreateOrganizationFormValues>({
    resolver: zodResolver(schema),
    defaultValues: CREATE_ORGANIZATION_DEFAULTS,
  })

  const createMutation = useMutation({
    mutationFn: createOrganization,
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: organizationKeys.lists() })
      toast.success(
        t('Organization {{name}} was created', {
          name: result.organization.name,
        })
      )
      form.reset(CREATE_ORGANIZATION_DEFAULTS)
      props.onOpenChange(false)
    },
    onError: handleServerError,
  })

  const handleOpenChange = (open: boolean) => {
    if (createMutation.isPending) return
    if (!open) form.reset(CREATE_ORGANIZATION_DEFAULTS)
    props.onOpenChange(open)
  }

  const onSubmit = (values: CreateOrganizationFormValues) => {
    createMutation.mutate(values)
  }

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent className='max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Create organization')}</DialogTitle>
          <DialogDescription>
            {t(
              'Create the organization and its Owner account in one operation.'
            )}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            id='create-organization-form'
            onSubmit={form.handleSubmit(onSubmit)}
          >
            <fieldset disabled={createMutation.isPending}>
              <FieldGroup>
                <FormField
                  control={form.control}
                  name='name'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Organization name')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          autoFocus
                          autoComplete='organization'
                          placeholder={t('Enter organization name')}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <div className='grid gap-5 sm:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='owner_username'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Owner username')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            autoComplete='off'
                            placeholder={t('Enter owner username')}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='owner_display_name'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Owner display name')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            autoComplete='off'
                            placeholder={t('Defaults to username')}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <FormField
                  control={form.control}
                  name='owner_email'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Owner email')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='email'
                          autoComplete='off'
                          placeholder={t('Optional email address')}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='owner_password'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Initial Owner password')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='password'
                          autoComplete='new-password'
                          placeholder={t('8-20 characters')}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'The Owner can change this password after signing in.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='allow_member_topup'
                  render={({ field }) => (
                    <FormItem className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                      <div className='flex min-w-0 flex-col gap-1'>
                        <FormLabel htmlFor='allow-member-topup'>
                          {t('Allow member top-ups')}
                        </FormLabel>
                        <FormDescription>
                          {t(
                            'Members can add funds to their own single balance when enabled.'
                          )}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          id='allow-member-topup'
                          checked={field.value}
                          onCheckedChange={field.onChange}
                          aria-label={t('Allow member top-ups')}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </FieldGroup>
            </fieldset>
          </form>
        </Form>

        <DialogFooter>
          <DialogClose render={<Button variant='outline' />}>
            {t('Cancel')}
          </DialogClose>
          <Button
            type='submit'
            form='create-organization-form'
            disabled={createMutation.isPending}
          >
            {createMutation.isPending && <Spinner data-icon='inline-start' />}
            {createMutation.isPending
              ? t('Creating...')
              : t('Create organization')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
