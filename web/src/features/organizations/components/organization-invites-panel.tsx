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
import {
  Add01Icon,
  BlockedIcon,
  Ticket01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  disableOrganizationInvite,
  listOrganizationInvites,
  organizationKeys,
} from '../api'
import type { OrganizationInvite } from '../types'
import { CreateOrganizationInviteDialog } from './create-organization-invite-dialog'
import { InviteStatusBadge } from './organization-badges'
import { OrganizationQueryError } from './organization-query-error'
import { PaginationControls } from './pagination-controls'

const PAGE_SIZE = 10

export function OrganizationInvitesPanel(props: { organizationId: number }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [createOpen, setCreateOpen] = useState(false)
  const [disableCandidate, setDisableCandidate] =
    useState<OrganizationInvite | null>(null)
  const queryParams = {
    organizationId: props.organizationId,
    page,
    pageSize: PAGE_SIZE,
  }
  const invitesQuery = useQuery({
    queryKey: organizationKeys.inviteList(queryParams),
    queryFn: () => listOrganizationInvites(queryParams),
    placeholderData: keepPreviousData,
  })
  const disableMutation = useMutation({
    mutationFn: (inviteId: number) =>
      disableOrganizationInvite(props.organizationId, inviteId),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: organizationKeys.invites(props.organizationId),
      })
      setDisableCandidate(null)
      toast.success(t('Organization invite disabled'))
    },
    onError: handleServerError,
  })
  const invites = invitesQuery.data?.items ?? []
  const total = invitesQuery.data?.total ?? 0
  const hasLoadError = invitesQuery.isError && !invitesQuery.data

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <p className='text-muted-foreground text-sm'>
          {t('Invite codes can only create ordinary organization members.')}
        </p>
        <Button type='button' size='sm' onClick={() => setCreateOpen(true)}>
          <HugeiconsIcon
            icon={Add01Icon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Create invite')}
        </Button>
      </div>

      {invitesQuery.isLoading && (
        <div className='flex flex-col gap-2'>
          {Array.from({ length: 5 }, (_, index) => (
            <Skeleton key={index} className='h-14 w-full' />
          ))}
        </div>
      )}

      {hasLoadError && (
        <OrganizationQueryError onRetry={() => void invitesQuery.refetch()} />
      )}

      {!invitesQuery.isLoading && !hasLoadError && invites.length === 0 && (
        <Empty className='border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={Ticket01Icon} strokeWidth={2} />
            </EmptyMedia>
            <EmptyTitle>{t('No organization invites')}</EmptyTitle>
            <EmptyDescription>
              {t('Create an invite when this organization needs new members.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}

      {!invitesQuery.isLoading && !hasLoadError && invites.length > 0 && (
        <>
          <div className='overflow-x-auto rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Code prefix')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Usage')}</TableHead>
                  <TableHead>{t('Expiration')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {invites.map((invite) => (
                  <TableRow key={invite.id}>
                    <TableCell>
                      <code className='font-medium'>
                        {invite.code_prefix}...
                      </code>
                    </TableCell>
                    <TableCell>
                      <InviteStatusBadge status={invite.status} />
                    </TableCell>
                    <TableCell>
                      {invite.max_uses > 0
                        ? t('{{used}} / {{max}}', {
                            used: invite.used_count,
                            max: invite.max_uses,
                          })
                        : t('{{used}} / unlimited', {
                            used: invite.used_count,
                          })}
                    </TableCell>
                    <TableCell>
                      {invite.expires_at > 0
                        ? formatTimestampToDate(invite.expires_at)
                        : t('Never')}
                    </TableCell>
                    <TableCell className='text-right'>
                      <Button
                        type='button'
                        variant='destructive'
                        size='sm'
                        disabled={
                          invite.status !== 'active' ||
                          disableMutation.isPending
                        }
                        onClick={() => setDisableCandidate(invite)}
                      >
                        <HugeiconsIcon
                          icon={BlockedIcon}
                          strokeWidth={2}
                          data-icon='inline-start'
                        />
                        {t('Disable')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <PaginationControls
            page={page}
            pageSize={PAGE_SIZE}
            total={total}
            disabled={invitesQuery.isFetching}
            onPageChange={setPage}
          />
        </>
      )}

      <CreateOrganizationInviteDialog
        organizationId={props.organizationId}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />

      <AlertDialog
        open={disableCandidate !== null}
        onOpenChange={(open) => {
          if (!open && !disableMutation.isPending) setDisableCandidate(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <HugeiconsIcon icon={BlockedIcon} strokeWidth={2} />
            </AlertDialogMedia>
            <AlertDialogTitle>{t('Disable invite?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'New registrations will no longer be able to use this organization invite.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={disableMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={disableMutation.isPending || disableCandidate === null}
              onClick={() => {
                if (disableCandidate) {
                  disableMutation.mutate(disableCandidate.id)
                }
              }}
            >
              {disableMutation.isPending && (
                <Spinner data-icon='inline-start' />
              )}
              {t('Disable invite')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
