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

import { PasswordInput } from '@/components/password-input'
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { handleServerError } from '@/lib/handle-server-error'

import { organizationKeys, provisionOrganizationMember } from '../api'
import {
  getProvisionOrganizationMemberSchema,
  ORGANIZATION_MEMBER_ROLE_OPTIONS,
  PROVISION_ORGANIZATION_MEMBER_DEFAULTS,
  type ProvisionOrganizationMemberFormValues,
} from '../lib/organization-member-form'

type ProvisionOrganizationMemberDialogProps = {
  organizationId: number
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ProvisionOrganizationMemberDialog(
  props: ProvisionOrganizationMemberDialogProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const schema = useMemo(() => getProvisionOrganizationMemberSchema(t), [t])
  const form = useForm<ProvisionOrganizationMemberFormValues>({
    resolver: zodResolver(schema),
    defaultValues: PROVISION_ORGANIZATION_MEMBER_DEFAULTS,
  })
  const roleItems = useMemo(
    () => [
      { value: 'admin', label: t('Organization admin') },
      { value: 'member', label: t('Member') },
    ],
    [t]
  )

  const provisionMutation = useMutation({
    mutationFn: (values: ProvisionOrganizationMemberFormValues) =>
      provisionOrganizationMember(props.organizationId, values),
    onSuccess: (member) => {
      void queryClient.invalidateQueries({
        queryKey: organizationKeys.members(props.organizationId),
      })
      void queryClient.invalidateQueries({ queryKey: organizationKeys.lists() })
      toast.success(
        t('Organization account {{username}} was created', {
          username: member.username,
        })
      )
      form.reset(PROVISION_ORGANIZATION_MEMBER_DEFAULTS)
      props.onOpenChange(false)
    },
    onError: handleServerError,
  })

  const handleOpenChange = (open: boolean) => {
    if (provisionMutation.isPending) return
    if (!open) form.reset(PROVISION_ORGANIZATION_MEMBER_DEFAULTS)
    props.onOpenChange(open)
  }

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent className='max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Create organization account')}</DialogTitle>
          <DialogDescription>
            {t(
              'Only platform administrators can create organization administrators. Owner accounts are assigned separately.'
            )}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            id='provision-organization-member-form'
            onSubmit={form.handleSubmit((values) =>
              provisionMutation.mutate(values)
            )}
          >
            <fieldset disabled={provisionMutation.isPending}>
              <FieldGroup>
                <FormField
                  control={form.control}
                  name='organization_role'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Organization role')}</FormLabel>
                      <Select
                        items={roleItems}
                        value={field.value}
                        onValueChange={(value) => {
                          if (
                            value &&
                            ORGANIZATION_MEMBER_ROLE_OPTIONS.includes(value)
                          ) {
                            field.onChange(value)
                          }
                        }}
                      >
                        <FormControl>
                          <SelectTrigger aria-label={t('Organization role')}>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            {roleItems.map((item) => (
                              <SelectItem key={item.value} value={item.value}>
                                {item.label}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        {t(
                          'Organization admin creation and role assignment remain platform-controlled.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <div className='grid gap-5 sm:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='username'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Username')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            autoFocus
                            autoComplete='off'
                            placeholder={t('Enter username')}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='display_name'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Display Name')}</FormLabel>
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
                  name='email'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Email')}</FormLabel>
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
                  name='password'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Initial password')}</FormLabel>
                      <FormControl>
                        <PasswordInput
                          {...field}
                          autoComplete='new-password'
                          placeholder={t('8-20 characters')}
                        />
                      </FormControl>
                      <FormMessage />
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
            form='provision-organization-member-form'
            disabled={provisionMutation.isPending}
          >
            {provisionMutation.isPending && (
              <Spinner data-icon='inline-start' />
            )}
            {provisionMutation.isPending
              ? t('Creating...')
              : t('Create account')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
