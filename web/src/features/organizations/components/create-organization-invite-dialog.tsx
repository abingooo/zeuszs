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
import { Copy01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Spinner } from '@/components/ui/spinner'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { handleServerError } from '@/lib/handle-server-error'

import { createOrganizationInvite, organizationKeys } from '../api'
import {
  CREATE_INVITE_DEFAULTS,
  getCreateInviteSchema,
  type CreateInviteFormValues,
} from '../lib/invite-form'
import {
  createTenantOrganizationInvite,
  tenantOrganizationKeys,
} from '../tenant-api'

type CreateOrganizationInviteDialogProps = {
  organizationId: number
  scope?: 'platform' | 'tenant'
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CreateOrganizationInviteDialog(
  props: CreateOrganizationInviteDialogProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createdCode, setCreatedCode] = useState('')
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: true })
  const schema = useMemo(() => getCreateInviteSchema(t), [t])
  const form = useForm<CreateInviteFormValues>({
    resolver: zodResolver(schema),
    defaultValues: CREATE_INVITE_DEFAULTS,
  })
  const scope = props.scope ?? 'platform'
  const createMutation = useMutation({
    mutationFn: (values: CreateInviteFormValues) => {
      const input = {
        code: values.code || undefined,
        max_uses: values.max_uses,
        expires_at: values.expires_at
          ? Math.floor(new Date(values.expires_at).getTime() / 1000)
          : 0,
      }
      if (scope === 'tenant') {
        return createTenantOrganizationInvite(input)
      }
      return createOrganizationInvite(props.organizationId, input)
    },
    onSuccess: (invite) => {
      const queryKey =
        scope === 'tenant'
          ? tenantOrganizationKeys.invites()
          : organizationKeys.invites(props.organizationId)
      void queryClient.invalidateQueries({ queryKey })
      if (scope === 'tenant') {
        void queryClient.invalidateQueries({
          queryKey: tenantOrganizationKeys.audit(),
        })
      }
      setCreatedCode(invite.code ?? '')
    },
    onError: handleServerError,
  })

  const handleOpenChange = (open: boolean) => {
    if (createMutation.isPending) return
    if (!open) {
      form.reset(CREATE_INVITE_DEFAULTS)
      setCreatedCode('')
    }
    props.onOpenChange(open)
  }

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Create organization invite')}</DialogTitle>
          <DialogDescription>
            {t('Organization invites always register ordinary members.')}
          </DialogDescription>
        </DialogHeader>

        {createdCode ? (
          <Alert>
            <AlertTitle>{t('Invite created')}</AlertTitle>
            <AlertDescription className='flex flex-col gap-3'>
              <span>
                {t('Copy this code now. It will not be shown again.')}
              </span>
              <InputGroup>
                <InputGroupInput
                  readOnly
                  value={createdCode}
                  aria-label={t('Organization invite code')}
                />
                <InputGroupAddon align='inline-end'>
                  <InputGroupButton
                    type='button'
                    size='icon-xs'
                    onClick={() => void copyToClipboard(createdCode)}
                    aria-label={t('Copy invite code')}
                  >
                    <HugeiconsIcon
                      icon={
                        copiedText === createdCode ? Tick02Icon : Copy01Icon
                      }
                      strokeWidth={2}
                    />
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
            </AlertDescription>
          </Alert>
        ) : (
          <Form {...form}>
            <form
              id='create-organization-invite-form'
              onSubmit={form.handleSubmit((values) =>
                createMutation.mutate(values)
              )}
            >
              <fieldset disabled={createMutation.isPending}>
                <FieldGroup>
                  <FormField
                    control={form.control}
                    name='code'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Custom invite code')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            autoComplete='off'
                            placeholder={t('Leave empty to generate securely')}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Use 8-64 characters when setting a custom code.')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='max_uses'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Maximum uses')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type='number'
                            min={0}
                            max={1_000_000_000}
                            step={1}
                            onChange={(event) =>
                              field.onChange(event.target.valueAsNumber)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Use 0 for no usage limit.')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='expires_at'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Expiration')}</FormLabel>
                        <FormControl>
                          <Input {...field} type='datetime-local' />
                        </FormControl>
                        <FormDescription>
                          {t('Leave empty for no expiration.')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </FieldGroup>
              </fieldset>
            </form>
          </Form>
        )}

        <DialogFooter>
          {createdCode ? (
            <DialogClose render={<Button />}>{t('Done')}</DialogClose>
          ) : (
            <>
              <DialogClose render={<Button variant='outline' />}>
                {t('Cancel')}
              </DialogClose>
              <Button
                type='submit'
                form='create-organization-invite-form'
                disabled={createMutation.isPending}
              >
                {createMutation.isPending && (
                  <Spinner data-icon='inline-start' />
                )}
                {createMutation.isPending
                  ? t('Creating...')
                  : t('Create invite')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
