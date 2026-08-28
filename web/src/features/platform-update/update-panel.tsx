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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import {
  AlertCircle,
  CheckCircle2,
  Download,
  ExternalLink,
  RefreshCcw,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Markdown } from '@/components/ui/markdown'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { SettingsSection } from '@/features/system-settings/components/settings-section'
import { formatTimestampToDate } from '@/lib/format'

import { getPlatformUpdate, triggerPlatformUpdate } from './api'
import type { PlatformUpdateData, PlatformUpdateResponse } from './types'

const PLATFORM_UPDATE_QUERY_KEY = ['zeuszs', 'platform-update'] as const
const UPDATE_POLL_INTERVAL_MS = 5_000

function getResponseData(
  response: PlatformUpdateResponse,
  fallbackMessage: string
): PlatformUpdateData {
  if (!response.success || !response.data) {
    throw new Error(response.message || fallbackMessage)
  }
  return response.data
}

function getUpdateErrorDetails(error: unknown): {
  code?: string
  message?: string
} {
  if (axios.isAxiosError<PlatformUpdateResponse>(error)) {
    return {
      code: error.response?.data?.code,
      message: error.response?.data?.message || error.message,
    }
  }
  if (error instanceof Error) return { message: error.message }
  return {}
}

function PlatformUpdateLoading() {
  const { t } = useTranslation()

  return (
    <div
      className='flex flex-col gap-5'
      aria-label={t('Loading platform update')}
    >
      <div className='grid gap-4 sm:grid-cols-2'>
        <Skeleton className='h-24 w-full' />
        <Skeleton className='h-24 w-full' />
      </div>
      <Skeleton className='h-20 w-full' />
      <Skeleton className='h-52 w-full' />
    </div>
  )
}

export function PlatformUpdatePanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [confirmationOpen, setConfirmationOpen] = useState(false)
  const [targetVersion, setTargetVersion] = useState<string | null>(null)
  const [completedVersion, setCompletedVersion] = useState<string | null>(null)
  const [updateError, setUpdateError] = useState<string | null>(null)

  const updateQuery = useQuery({
    queryKey: PLATFORM_UPDATE_QUERY_KEY,
    queryFn: async () =>
      getResponseData(
        await getPlatformUpdate(),
        t('Failed to check for platform updates')
      ),
    refetchInterval: (query) => {
      const data = query.state.data
      return targetVersion || data?.updater_status?.status === 'running'
        ? UPDATE_POLL_INTERVAL_MS
        : false
    },
    refetchIntervalInBackground: true,
    retry: false,
  })

  const updateMutation = useMutation({
    mutationFn: async () =>
      getResponseData(
        await triggerPlatformUpdate(),
        t('Failed to start platform update')
      ),
    onSuccess: (data) => {
      setConfirmationOpen(false)
      setUpdateError(null)
      setCompletedVersion(null)
      setTargetVersion(data.latest_release.tag_name)
      queryClient.setQueryData(PLATFORM_UPDATE_QUERY_KEY, data)
      void queryClient.invalidateQueries({
        queryKey: PLATFORM_UPDATE_QUERY_KEY,
      })
    },
    onError: (error) => {
      setConfirmationOpen(false)
      const details = getUpdateErrorDetails(error)
      if (
        details.code === 'ZEUSZS_UPDATE_IN_PROGRESS' &&
        updateQuery.data?.latest_release.tag_name
      ) {
        setUpdateError(null)
        setTargetVersion(updateQuery.data.latest_release.tag_name)
        return
      }
      setUpdateError(details.message || t('Failed to start platform update'))
    },
  })

  const currentVersion = updateQuery.data?.current_version
  const updateAvailable = updateQuery.data?.update_available
  const updaterStatus = updateQuery.data?.updater_status
  const updaterState = updaterStatus?.status
  const updaterTag = updaterStatus?.tag
  const updaterVersionSucceeded =
    updaterState === 'succeeded' &&
    Boolean(updaterTag) &&
    updaterTag === currentVersion
  const targetUpdateCompleted =
    updaterVersionSucceeded &&
    Boolean(targetVersion) &&
    updaterTag === targetVersion

  useEffect(() => {
    if (updateAvailable) {
      setCompletedVersion(null)
    }
    if (updaterState === 'failed') {
      setTargetVersion(null)
      return
    }
    if (!targetUpdateCompleted || !updaterTag) {
      return
    }
    const version = updaterTag
    setCompletedVersion(version)
    setTargetVersion(null)
    setUpdateError(null)
    toast.success(t('Platform update completed: {{version}}', { version }))
  }, [
    t,
    targetVersion,
    targetUpdateCompleted,
    updateAvailable,
    updaterState,
    updaterTag,
  ])

  if (updateQuery.isLoading) {
    return <PlatformUpdateLoading />
  }

  if (updateQuery.isError && !updateQuery.data) {
    const details = getUpdateErrorDetails(updateQuery.error)
    return (
      <ErrorState
        title={t('Failed to check for platform updates')}
        description={details.message || t('Please try again later.')}
        onRetry={() => void updateQuery.refetch()}
      />
    )
  }

  const data = updateQuery.data
  if (!data) return null

  const release = data.latest_release
  const hostUpdateRunning = data.updater_status?.status === 'running'
  const hostUpdateFailed = data.updater_status?.status === 'failed'
  const hostUpdateSucceeded =
    updaterVersionSucceeded && data.update_available === false
  const isUpdating =
    hostUpdateRunning ||
    Boolean(targetVersion && !hostUpdateFailed && !hostUpdateSucceeded)
  const activeTargetVersion =
    targetVersion || data.updater_status?.tag || release.tag_name
  const canUpdate =
    data.update_available &&
    data.updater_configured &&
    data.updater_reachable &&
    !isUpdating
  const publishedAt = release.published_at
    ? formatTimestampToDate(
        new Date(release.published_at).getTime(),
        'milliseconds'
      )
    : t('Unknown')

  let statusNotice = (
    <Alert>
      <CheckCircle2 />
      <AlertTitle>{t('Platform is up to date')}</AlertTitle>
      <AlertDescription>
        {t('The installed version matches the latest available release.')}
      </AlertDescription>
    </Alert>
  )
  if (data.update_available) {
    statusNotice = (
      <Alert>
        <Download />
        <AlertTitle>{t('A platform update is available')}</AlertTitle>
        <AlertDescription>
          {t('Version {{version}} is ready to install.', {
            version: release.tag_name,
          })}
        </AlertDescription>
      </Alert>
    )
  }
  if (!data.updater_configured) {
    statusNotice = (
      <Alert variant='destructive'>
        <AlertCircle />
        <AlertTitle>{t('Platform updater is not configured')}</AlertTitle>
        <AlertDescription>
          {t(
            'Automatic installation is unavailable on this host. Configure the host updater before starting an update.'
          )}
        </AlertDescription>
      </Alert>
    )
  }
  if (data.updater_configured && !data.updater_reachable) {
    statusNotice = (
      <Alert variant='destructive'>
        <AlertCircle />
        <AlertTitle>{t('Platform updater is unavailable')}</AlertTitle>
        <AlertDescription>
          {t(
            'The host updater is configured but cannot be reached. Restore the updater service before starting an update.'
          )}
        </AlertDescription>
      </Alert>
    )
  }
  if (!data.update_available && (completedVersion || hostUpdateSucceeded)) {
    const version =
      completedVersion || data.updater_status?.tag || data.current_version
    statusNotice = (
      <Alert>
        <CheckCircle2 />
        <AlertTitle>{t('Platform update completed')}</AlertTitle>
        <AlertDescription>
          {t('The platform is now running {{version}}.', {
            version,
          })}
        </AlertDescription>
      </Alert>
    )
  }
  if (updateError) {
    statusNotice = (
      <Alert variant='destructive'>
        <AlertCircle />
        <AlertTitle>{t('Platform update failed')}</AlertTitle>
        <AlertDescription>{updateError}</AlertDescription>
      </Alert>
    )
  }
  if (hostUpdateFailed) {
    statusNotice = (
      <Alert variant='destructive'>
        <AlertCircle />
        <AlertTitle>{t('Platform update failed')}</AlertTitle>
        <AlertDescription className='flex flex-col gap-1'>
          <span>
            {data.updater_status?.error ||
              t('The host updater reported a failure.')}
          </span>
          {data.updater_status?.step && (
            <span>
              {t('Update step')}: {data.updater_status.step}
            </span>
          )}
        </AlertDescription>
      </Alert>
    )
  }
  if (isUpdating) {
    statusNotice = (
      <Alert>
        <Spinner />
        <AlertTitle>{t('Platform update in progress')}</AlertTitle>
        <AlertDescription className='flex flex-col gap-1'>
          <span>
            {t(
              'The update to {{version}} was accepted. This page will check every 5 seconds and confirm when the service returns on the new version.',
              { version: activeTargetVersion }
            )}
          </span>
          {data.updater_status?.step && (
            <span>
              {t('Update step')}: {data.updater_status.step}
            </span>
          )}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <SettingsSection title={t('Update status')} className='max-w-5xl'>
      <div className='flex flex-col gap-5'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='flex flex-wrap items-center gap-2'>
            <Badge variant={data.update_available ? 'warning' : 'secondary'}>
              {data.update_available ? t('Update available') : t('Up to date')}
            </Badge>
            <span className='text-muted-foreground text-sm'>
              {data.repository}
            </span>
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={() => void updateQuery.refetch()}
              disabled={updateQuery.isFetching || updateMutation.isPending}
            >
              {updateQuery.isFetching ? (
                <Spinner data-icon='inline-start' />
              ) : (
                <RefreshCcw data-icon='inline-start' />
              )}
              {updateQuery.isFetching ? t('Checking...') : t('Refresh')}
            </Button>
            {data.update_available && (
              <Button
                type='button'
                onClick={() => setConfirmationOpen(true)}
                disabled={!canUpdate || updateMutation.isPending}
              >
                {updateMutation.isPending ? (
                  <Spinner data-icon='inline-start' />
                ) : (
                  <Download data-icon='inline-start' />
                )}
                {isUpdating ? t('Updating...') : t('Update now')}
              </Button>
            )}
          </div>
        </div>

        {statusNotice}

        <dl className='grid overflow-hidden rounded-lg border sm:grid-cols-2 [&>div]:min-w-0 [&>div]:p-4 sm:[&>div:first-child]:border-r'>
          <div className='border-b sm:border-b-0'>
            <dt className='text-muted-foreground text-xs'>
              {t('Current version')}
            </dt>
            <dd className='mt-1 truncate font-mono text-lg font-semibold'>
              {data.current_version || t('Unknown')}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Latest version')}
            </dt>
            <dd className='mt-1 flex min-w-0 flex-wrap items-center gap-2'>
              <span className='truncate font-mono text-lg font-semibold'>
                {release.tag_name}
              </span>
              {release.prerelease && (
                <Badge variant='warning'>{t('Prerelease')}</Badge>
              )}
            </dd>
          </div>
        </dl>

        <section className='flex min-h-0 flex-col gap-3 border-t pt-5'>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <div className='min-w-0'>
              <h3 className='truncate text-sm font-semibold'>
                {release.name || release.tag_name}
              </h3>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('Published {{date}}', { date: publishedAt })}
              </p>
            </div>
            {release.html_url && (
              <Button
                variant='outline'
                size='sm'
                render={
                  <a
                    href={release.html_url}
                    target='_blank'
                    rel='noopener noreferrer'
                  />
                }
              >
                <ExternalLink data-icon='inline-start' />
                {t('Open release')}
              </Button>
            )}
          </div>
          <div className='bg-muted/20 max-h-[28rem] overflow-auto rounded-lg border p-4'>
            {release.body ? (
              <Markdown>{release.body}</Markdown>
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t('No release notes provided.')}
              </p>
            )}
          </div>
        </section>
      </div>

      <AlertDialog open={confirmationOpen} onOpenChange={setConfirmationOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Confirm platform update')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Install {{version}} now? The service will restart and may be unavailable briefly.',
                { version: release.tag_name }
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={updateMutation.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => updateMutation.mutate()}
              disabled={updateMutation.isPending}
            >
              {updateMutation.isPending && <Spinner data-icon='inline-start' />}
              {t('Start update')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsSection>
  )
}
