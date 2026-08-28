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
  DashboardSpeed01Icon,
  Key01Icon,
  MoneyAdd01Icon,
  MoneyRemove01Icon,
  MoreHorizontalIcon,
  Search01Icon,
  UserBlock01Icon,
  UserCheck01Icon,
  UserMultipleIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { type FormEvent, useState } from 'react'
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'
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
import { useIdVisibility } from '@/hooks/use-id-visibility'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  disableTenantMemberTokens,
  listTenantOrganizationMembers,
  tenantOrganizationKeys,
  updateTenantMemberStatus,
} from '../tenant-api'
import type {
  OrganizationMember,
  OrganizationMemberStatus,
  TenantOrganizationSummary,
} from '../types'
import { MemberStatusBadge, OrganizationRoleBadge } from './organization-badges'
import { OrganizationQueryError } from './organization-query-error'
import { PaginationControls } from './pagination-controls'
import {
  TenantMemberLimitDialog,
  TenantMemberQuotaDialog,
} from './tenant-member-quota-dialogs'

const PAGE_SIZE = 10

type QuotaAction = {
  member: OrganizationMember
  mode: 'allocate' | 'recover'
}

type ConfirmationAction = {
  member: OrganizationMember
  type: 'status' | 'tokens'
}

export function TenantOrganizationMembersPanel(props: {
  summary: TenantOrganizationSummary
}) {
  const { t } = useTranslation()
  const showInternalIds = useIdVisibility()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [searchInput, setSearchInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [quotaAction, setQuotaAction] = useState<QuotaAction | null>(null)
  const [limitMember, setLimitMember] = useState<OrganizationMember | null>(
    null
  )
  const [confirmation, setConfirmation] = useState<ConfirmationAction | null>(
    null
  )
  const queryParams = {
    organizationId: props.summary.organization_id,
    page,
    pageSize: PAGE_SIZE,
    keyword,
  }
  const membersQuery = useQuery({
    queryKey: tenantOrganizationKeys.memberList(queryParams),
    queryFn: () => listTenantOrganizationMembers(queryParams),
    placeholderData: keepPreviousData,
  })
  const statusMutation = useMutation({
    mutationFn: (input: { userId: number; status: OrganizationMemberStatus }) =>
      updateTenantMemberStatus(input.userId, input.status),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: tenantOrganizationKeys.all,
      })
      setConfirmation(null)
      toast.success(t('Member status updated'))
    },
    onError: handleServerError,
  })
  const tokenMutation = useMutation({
    mutationFn: disableTenantMemberTokens,
    onSuccess: (result) => {
      void queryClient.invalidateQueries({
        queryKey: tenantOrganizationKeys.audit(),
      })
      setConfirmation(null)
      toast.success(
        t('{{count}} API keys disabled', {
          count: result.disabled_token_count,
        })
      )
    },
    onError: handleServerError,
  })

  const handleSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setKeyword(searchInput.trim())
    setPage(1)
  }

  const members = membersQuery.data?.items ?? []
  const total = membersQuery.data?.total ?? 0
  const hasLoadError = membersQuery.isError && !membersQuery.data
  const confirmationPending =
    statusMutation.isPending || tokenMutation.isPending
  const confirmationIsTokenAction = confirmation?.type === 'tokens'
  const confirmationTitle = confirmationIsTokenAction
    ? t('Disable all member API keys?')
    : t('Change member status?')
  const confirmationDescription = confirmationIsTokenAction
    ? t('All currently enabled API keys owned by this member will be disabled.')
    : t('This changes whether the member can access the organization.')

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3'>
      <form onSubmit={handleSearch} className='w-full sm:max-w-sm'>
        <InputGroup>
          <InputGroupInput
            value={searchInput}
            onChange={(event) => setSearchInput(event.target.value)}
            placeholder={t('Search members')}
            aria-label={t('Search members')}
          />
          <InputGroupAddon align='inline-end'>
            <InputGroupButton
              type='submit'
              size='icon-xs'
              aria-label={t('Search')}
            >
              <HugeiconsIcon icon={Search01Icon} strokeWidth={2} />
            </InputGroupButton>
          </InputGroupAddon>
        </InputGroup>
      </form>

      {membersQuery.isLoading && (
        <div className='flex flex-col gap-2'>
          {Array.from({ length: 5 }, (_, index) => (
            <Skeleton key={index} className='h-14 w-full' />
          ))}
        </div>
      )}

      {hasLoadError && (
        <OrganizationQueryError onRetry={() => void membersQuery.refetch()} />
      )}

      {!membersQuery.isLoading && !hasLoadError && members.length === 0 && (
        <Empty className='border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={UserMultipleIcon} strokeWidth={2} />
            </EmptyMedia>
            <EmptyTitle>{t('No organization members found')}</EmptyTitle>
            <EmptyDescription>
              {t('Try another member search or page.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}

      {!membersQuery.isLoading && !hasLoadError && members.length > 0 && (
        <>
          <div className='overflow-x-auto rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Member')}</TableHead>
                  <TableHead>{t('Role and status')}</TableHead>
                  <TableHead>{t('Balance')}</TableHead>
                  <TableHead>{t('Recoverable')}</TableHead>
                  <TableHead>{t('Consumption limit')}</TableHead>
                  <TableHead className='w-12'>
                    <span className='sr-only'>{t('Actions')}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {members.map((member) => {
                  const isOrdinaryMember = member.organization_role === 'member'
                  return (
                    <TableRow key={member.user_id}>
                      <TableCell>
                        <div className='flex min-w-44 flex-col gap-0.5'>
                          <span className='truncate font-medium'>
                            {member.display_name || member.username}
                          </span>
                          <span className='text-muted-foreground truncate text-xs'>
                            @{member.username}
                            {showInternalIds && (
                              <> - {t('ID: {{id}}', { id: member.user_id })}</>
                            )}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className='flex flex-wrap gap-1.5'>
                          <OrganizationRoleBadge
                            role={member.organization_role}
                          />
                          <MemberStatusBadge
                            status={member.organization_status}
                          />
                        </div>
                      </TableCell>
                      <TableCell>{formatQuota(member.quota)}</TableCell>
                      <TableCell>
                        {formatQuota(member.recoverable_quota)}
                      </TableCell>
                      <TableCell>
                        {member.consumption_limit === undefined
                          ? t('No limit')
                          : formatQuota(member.consumption_limit)}
                      </TableCell>
                      <TableCell>
                        {isOrdinaryMember && (
                          <DropdownMenu>
                            <DropdownMenuTrigger
                              render={
                                <Button
                                  type='button'
                                  variant='ghost'
                                  size='icon-sm'
                                  aria-label={t('Manage member')}
                                />
                              }
                            >
                              <HugeiconsIcon
                                icon={MoreHorizontalIcon}
                                strokeWidth={2}
                              />
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align='end'>
                              <DropdownMenuGroup>
                                <DropdownMenuItem
                                  onClick={() =>
                                    setQuotaAction({
                                      member,
                                      mode: 'allocate',
                                    })
                                  }
                                >
                                  <HugeiconsIcon
                                    icon={MoneyAdd01Icon}
                                    strokeWidth={2}
                                  />
                                  {t('Allocate quota')}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  disabled={member.recoverable_quota <= 0}
                                  onClick={() =>
                                    setQuotaAction({
                                      member,
                                      mode: 'recover',
                                    })
                                  }
                                >
                                  <HugeiconsIcon
                                    icon={MoneyRemove01Icon}
                                    strokeWidth={2}
                                  />
                                  {t('Recover quota')}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  onClick={() => setLimitMember(member)}
                                >
                                  <HugeiconsIcon
                                    icon={DashboardSpeed01Icon}
                                    strokeWidth={2}
                                  />
                                  {t('Set consumption limit')}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  onClick={() =>
                                    setConfirmation({ member, type: 'tokens' })
                                  }
                                >
                                  <HugeiconsIcon
                                    icon={Key01Icon}
                                    strokeWidth={2}
                                  />
                                  {t('Disable API keys')}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  onClick={() =>
                                    setConfirmation({ member, type: 'status' })
                                  }
                                >
                                  <HugeiconsIcon
                                    icon={
                                      member.organization_status === 'active'
                                        ? UserBlock01Icon
                                        : UserCheck01Icon
                                    }
                                    strokeWidth={2}
                                  />
                                  {member.organization_status === 'active'
                                    ? t('Disable member')
                                    : t('Enable member')}
                                </DropdownMenuItem>
                              </DropdownMenuGroup>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
          <PaginationControls
            page={page}
            pageSize={PAGE_SIZE}
            total={total}
            disabled={membersQuery.isFetching}
            onPageChange={setPage}
          />
        </>
      )}

      {quotaAction && (
        <TenantMemberQuotaDialog
          key={`${quotaAction.member.user_id}-${quotaAction.mode}`}
          member={quotaAction.member}
          mode={quotaAction.mode}
          poolQuota={props.summary.fund_quota ?? 0}
          open
          onOpenChange={(open) => {
            if (!open) setQuotaAction(null)
          }}
        />
      )}
      {limitMember && (
        <TenantMemberLimitDialog
          key={limitMember.user_id}
          member={limitMember}
          open
          onOpenChange={(open) => {
            if (!open) setLimitMember(null)
          }}
        />
      )}

      <AlertDialog
        open={confirmation !== null}
        onOpenChange={(open) => {
          if (!open && !confirmationPending) setConfirmation(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <HugeiconsIcon
                icon={confirmationIsTokenAction ? Key01Icon : UserBlock01Icon}
                strokeWidth={2}
              />
            </AlertDialogMedia>
            <AlertDialogTitle>{confirmationTitle}</AlertDialogTitle>
            <AlertDialogDescription>
              {confirmationDescription}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={confirmationPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={confirmationPending || confirmation === null}
              onClick={() => {
                if (!confirmation) return
                if (confirmation.type === 'tokens') {
                  tokenMutation.mutate(confirmation.member.user_id)
                  return
                }
                statusMutation.mutate({
                  userId: confirmation.member.user_id,
                  status:
                    confirmation.member.organization_status === 'active'
                      ? 'disabled'
                      : 'active',
                })
              }}
            >
              {confirmationPending && <Spinner data-icon='inline-start' />}
              {t('Confirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
