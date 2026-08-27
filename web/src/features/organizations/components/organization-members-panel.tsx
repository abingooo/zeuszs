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
  Search01Icon,
  UserMultipleIcon,
  UserSwitchIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { type FormEvent, useMemo, useState } from 'react'
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
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { handleServerError } from '@/lib/handle-server-error'

import {
  listOrganizationMembers,
  organizationKeys,
  transferOrganizationOwnership,
  updateOrganizationMemberRole,
} from '../api'
import type {
  Organization,
  OrganizationMember,
  OrganizationRole,
} from '../types'
import { MemberStatusBadge, OrganizationRoleBadge } from './organization-badges'
import { OrganizationQueryError } from './organization-query-error'
import { PaginationControls } from './pagination-controls'
import { ProvisionOrganizationMemberDialog } from './provision-organization-member-dialog'

const PAGE_SIZE = 10

type OrganizationMembersPanelProps = {
  organization: Organization
  onOrganizationChange: (organization: Organization) => void
}

export function OrganizationMembersPanel(props: OrganizationMembersPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [searchInput, setSearchInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [provisionOpen, setProvisionOpen] = useState(false)
  const [ownershipCandidate, setOwnershipCandidate] =
    useState<OrganizationMember | null>(null)
  const queryParams = {
    organizationId: props.organization.id,
    page,
    pageSize: PAGE_SIZE,
    keyword,
  }
  const membersQuery = useQuery({
    queryKey: organizationKeys.memberList(queryParams),
    queryFn: () => listOrganizationMembers(queryParams),
    placeholderData: keepPreviousData,
  })
  const roleItems = useMemo(
    () => [
      { value: 'member', label: t('Member') },
      { value: 'admin', label: t('Organization admin') },
    ],
    [t]
  )

  const roleMutation = useMutation({
    mutationFn: (input: {
      userId: number
      role: Exclude<OrganizationRole, 'owner'>
    }) =>
      updateOrganizationMemberRole(
        props.organization.id,
        input.userId,
        input.role
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: organizationKeys.members(props.organization.id),
      })
      void queryClient.invalidateQueries({ queryKey: organizationKeys.lists() })
      toast.success(t('Organization role updated'))
    },
    onError: handleServerError,
  })

  const ownershipMutation = useMutation({
    mutationFn: (userId: number) =>
      transferOrganizationOwnership(props.organization.id, userId),
    onSuccess: (organization) => {
      void queryClient.invalidateQueries({
        queryKey: organizationKeys.members(props.organization.id),
      })
      void queryClient.invalidateQueries({ queryKey: organizationKeys.lists() })
      props.onOrganizationChange({
        ...props.organization,
        ...organization,
      })
      setOwnershipCandidate(null)
      toast.success(t('Organization ownership transferred'))
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

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
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
        <Button type='button' size='sm' onClick={() => setProvisionOpen(true)}>
          <HugeiconsIcon
            icon={Add01Icon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Create account')}
        </Button>
      </div>

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
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Organization role')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {members.map((member) => {
                  const isOwner = member.organization_role === 'owner'
                  const isUpdatingRole =
                    roleMutation.isPending &&
                    roleMutation.variables?.userId === member.user_id

                  return (
                    <TableRow key={member.user_id}>
                      <TableCell>
                        <div className='flex min-w-48 flex-col gap-0.5'>
                          <span className='truncate font-medium'>
                            {member.display_name || member.username}
                          </span>
                          <span className='text-muted-foreground truncate text-xs'>
                            @{member.username} ·{' '}
                            {t('ID: {{id}}', { id: member.user_id })}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <MemberStatusBadge
                          status={member.organization_status}
                        />
                      </TableCell>
                      <TableCell>
                        {isOwner ? (
                          <OrganizationRoleBadge role='owner' />
                        ) : (
                          <Select
                            items={roleItems}
                            value={member.organization_role}
                            disabled={
                              isUpdatingRole || ownershipMutation.isPending
                            }
                            onValueChange={(value) =>
                              roleMutation.mutate({
                                userId: member.user_id,
                                role: value as Exclude<
                                  OrganizationRole,
                                  'owner'
                                >,
                              })
                            }
                          >
                            <SelectTrigger
                              className='w-40'
                              aria-label={t('Role for {{username}}', {
                                username: member.username,
                              })}
                            >
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent alignItemWithTrigger={false}>
                              <SelectGroup>
                                {roleItems.map((item) => (
                                  <SelectItem
                                    key={item.value}
                                    value={item.value}
                                  >
                                    {item.label}
                                  </SelectItem>
                                ))}
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        )}
                      </TableCell>
                      <TableCell className='text-right'>
                        {!isOwner && (
                          <Button
                            type='button'
                            variant='outline'
                            size='sm'
                            disabled={ownershipMutation.isPending}
                            onClick={() => setOwnershipCandidate(member)}
                          >
                            <HugeiconsIcon
                              icon={UserSwitchIcon}
                              strokeWidth={2}
                              data-icon='inline-start'
                            />
                            {t('Make Owner')}
                          </Button>
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

      <AlertDialog
        open={ownershipCandidate !== null}
        onOpenChange={(open) => {
          if (!open && !ownershipMutation.isPending) {
            setOwnershipCandidate(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <HugeiconsIcon icon={UserSwitchIcon} strokeWidth={2} />
            </AlertDialogMedia>
            <AlertDialogTitle>{t('Transfer ownership?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                '{{username}} will become the only Owner. The current Owner will become an organization admin.',
                { username: ownershipCandidate?.username ?? '' }
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={ownershipMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={
                ownershipMutation.isPending || ownershipCandidate === null
              }
              onClick={() => {
                if (ownershipCandidate) {
                  ownershipMutation.mutate(ownershipCandidate.user_id)
                }
              }}
            >
              {ownershipMutation.isPending && (
                <Spinner data-icon='inline-start' />
              )}
              {t('Transfer ownership')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <ProvisionOrganizationMemberDialog
        organizationId={props.organization.id}
        open={provisionOpen}
        onOpenChange={setProvisionOpen}
      />
    </div>
  )
}
