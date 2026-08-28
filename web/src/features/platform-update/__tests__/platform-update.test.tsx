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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { PropsWithChildren } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getPlatformUpdate, triggerPlatformUpdate } from '../api'
import type { PlatformUpdateData, PlatformUpdateResponse } from '../types'
import { PlatformUpdatePanel } from '../update-panel'

const mocks = vi.hoisted(() => ({
  toastSuccess: vi.fn(),
}))

vi.mock('../api', () => ({
  getPlatformUpdate: vi.fn(),
  triggerPlatformUpdate: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    success: mocks.toastSuccess,
  },
}))

function updateData(
  overrides: Partial<PlatformUpdateData> = {}
): PlatformUpdateData {
  return {
    repository: 'abingooo/zeuszs',
    current_version: 'zeuszs-v0.4.0',
    latest_release: {
      id: 5,
      tag_name: 'zeuszs-v0.5.0',
      name: 'ZeusZS v0.5.0',
      html_url: 'https://example.com/releases/v0.5.0',
      body: 'Release notes',
      published_at: '2026-08-28T08:00:00Z',
      prerelease: false,
    },
    update_available: true,
    updater_configured: true,
    updater_reachable: true,
    updater_status: null,
    ...overrides,
  }
}

function response(data: PlatformUpdateData): PlatformUpdateResponse {
  return { success: true, message: '', data }
}

function queryWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })

  return function Wrapper(props: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        {props.children}
      </QueryClientProvider>
    )
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('PlatformUpdatePanel', () => {
  test('checks automatically and shows the up-to-date state', async () => {
    const data = updateData({
      current_version: 'zeuszs-v0.5.0',
      update_available: false,
    })
    vi.mocked(getPlatformUpdate).mockResolvedValue(response(data))

    render(<PlatformUpdatePanel />, { wrapper: queryWrapper() })

    expect(await screen.findByText('Platform is up to date')).toBeVisible()
    expect(screen.getAllByText('zeuszs-v0.5.0')).toHaveLength(2)
    expect(getPlatformUpdate).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: 'Update now' })).toBeNull()
  })

  test('disables installation when the host updater is not configured', async () => {
    vi.mocked(getPlatformUpdate).mockResolvedValue(
      response(updateData({ updater_configured: false }))
    )

    render(<PlatformUpdatePanel />, { wrapper: queryWrapper() })

    expect(
      await screen.findByText('Platform updater is not configured')
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Update now' })).toBeDisabled()
  })

  test('disables installation when the configured host updater is unreachable', async () => {
    vi.mocked(getPlatformUpdate).mockResolvedValue(
      response(updateData({ updater_reachable: false }))
    )

    render(<PlatformUpdatePanel />, { wrapper: queryWrapper() })

    expect(
      await screen.findByText('Platform updater is unavailable')
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Update now' })).toBeDisabled()
  })

  test('restores a running host update after the page is refreshed', async () => {
    vi.mocked(getPlatformUpdate).mockResolvedValue(
      response(
        updateData({
          updater_status: {
            status: 'running',
            tag: 'zeuszs-v0.5.0',
            step: 'installing',
            started_at: '2026-08-28T08:05:00Z',
            finished_at: null,
          },
        })
      )
    )

    render(<PlatformUpdatePanel />, { wrapper: queryWrapper() })

    expect(await screen.findByText('Platform update in progress')).toBeVisible()
    expect(screen.getByText(/installing/)).toBeVisible()
    expect(screen.getByRole('button', { name: 'Updating...' })).toBeDisabled()
  })

  test('shows the host error and step when an update fails', async () => {
    vi.mocked(getPlatformUpdate).mockResolvedValue(
      response(
        updateData({
          updater_status: {
            status: 'failed',
            tag: 'zeuszs-v0.5.0',
            step: 'restart',
            error: 'service did not become healthy',
            started_at: '2026-08-28T08:05:00Z',
            finished_at: '2026-08-28T08:06:00Z',
          },
        })
      )
    )

    render(<PlatformUpdatePanel />, { wrapper: queryWrapper() })

    expect(await screen.findByText('Platform update failed')).toBeVisible()
    expect(screen.getByText('service did not become healthy')).toBeVisible()
    expect(screen.getByText(/restart/)).toBeVisible()
    expect(screen.getByRole('button', { name: 'Update now' })).toBeEnabled()
  })

  test('keeps the update in progress until the host updater reports success', async () => {
    const user = userEvent.setup()
    const beforeUpdate = updateData()
    const installedButRunning = updateData({
      current_version: 'zeuszs-v0.5.0',
      update_available: false,
      updater_status: {
        status: 'running',
        tag: 'zeuszs-v0.5.0',
        step: 'health-check',
        started_at: '2026-08-28T08:05:00Z',
        finished_at: null,
      },
    })
    vi.mocked(getPlatformUpdate)
      .mockResolvedValueOnce(response(beforeUpdate))
      .mockResolvedValue(response(installedButRunning))
    vi.mocked(triggerPlatformUpdate).mockResolvedValue(
      response({
        ...beforeUpdate,
        triggered_at: '2026-08-28T08:05:00Z',
        updater_status: {
          status: 'running',
          tag: 'zeuszs-v0.5.0',
          step: 'downloading',
          started_at: '2026-08-28T08:05:00Z',
          finished_at: null,
        },
      })
    )

    render(<PlatformUpdatePanel />, { wrapper: queryWrapper() })

    await user.click(await screen.findByRole('button', { name: 'Update now' }))
    await user.click(screen.getByRole('button', { name: 'Start update' }))

    expect(await screen.findByText('Platform update in progress')).toBeVisible()
    expect(screen.getByText(/health-check/)).toBeVisible()
    expect(screen.queryByText('Platform update completed')).toBeNull()
    expect(mocks.toastSuccess).not.toHaveBeenCalled()
  })

  test('confirms an update after the host updater reports success', async () => {
    const user = userEvent.setup()
    const beforeUpdate = updateData()
    const afterUpdate = updateData({
      current_version: 'zeuszs-v0.5.0',
      update_available: false,
      updater_status: {
        status: 'succeeded',
        tag: 'zeuszs-v0.5.0',
        step: 'completed',
        started_at: '2026-08-28T08:05:00Z',
        finished_at: '2026-08-28T08:06:00Z',
      },
    })
    vi.mocked(getPlatformUpdate)
      .mockResolvedValueOnce(response(beforeUpdate))
      .mockResolvedValue(response(afterUpdate))
    vi.mocked(triggerPlatformUpdate).mockResolvedValue(
      response({
        ...beforeUpdate,
        triggered_at: '2026-08-28T08:05:00Z',
        updater_status: {
          status: 'running',
          tag: 'zeuszs-v0.5.0',
          step: 'downloading',
          started_at: '2026-08-28T08:05:00Z',
          finished_at: null,
        },
      })
    )

    render(<PlatformUpdatePanel />, { wrapper: queryWrapper() })

    await user.click(await screen.findByRole('button', { name: 'Update now' }))
    expect(screen.getByText('Confirm platform update')).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'Start update' }))

    await waitFor(() => {
      expect(triggerPlatformUpdate).toHaveBeenCalledTimes(1)
    })
    expect(await screen.findByText('Platform update completed')).toBeVisible()
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      'Platform update completed: zeuszs-v0.5.0'
    )
  })

  test('shows a newer release when the persisted success belongs to the installed version', async () => {
    vi.mocked(getPlatformUpdate).mockResolvedValue(
      response(
        updateData({
          current_version: 'zeuszs-v0.5.0',
          latest_release: {
            id: 6,
            tag_name: 'zeuszs-v0.6.0',
            name: 'ZeusZS v0.6.0',
            html_url: 'https://example.com/releases/v0.6.0',
            body: 'New release notes',
            published_at: '2026-08-29T08:00:00Z',
            prerelease: false,
          },
          update_available: true,
          updater_status: {
            status: 'succeeded',
            tag: 'zeuszs-v0.5.0',
            step: 'completed',
            started_at: '2026-08-28T08:05:00Z',
            finished_at: '2026-08-28T08:06:00Z',
          },
        })
      )
    )

    render(<PlatformUpdatePanel />, { wrapper: queryWrapper() })

    expect(
      await screen.findByText('A platform update is available')
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Update now' })).toBeEnabled()
    expect(screen.queryByText('Platform update completed')).toBeNull()
  })

  test('allows the next release after the requested update succeeds', async () => {
    const user = userEvent.setup()
    const beforeUpdate = updateData()
    const nextReleaseAvailable = updateData({
      current_version: 'zeuszs-v0.5.0',
      latest_release: {
        id: 6,
        tag_name: 'zeuszs-v0.6.0',
        name: 'ZeusZS v0.6.0',
        html_url: 'https://example.com/releases/v0.6.0',
        body: 'New release notes',
        published_at: '2026-08-29T08:00:00Z',
        prerelease: false,
      },
      update_available: true,
      updater_status: {
        status: 'succeeded',
        tag: 'zeuszs-v0.5.0',
        step: 'completed',
        started_at: '2026-08-28T08:05:00Z',
        finished_at: '2026-08-28T08:06:00Z',
      },
    })
    vi.mocked(getPlatformUpdate)
      .mockResolvedValueOnce(response(beforeUpdate))
      .mockResolvedValue(response(nextReleaseAvailable))
    vi.mocked(triggerPlatformUpdate).mockResolvedValue(
      response({
        ...beforeUpdate,
        triggered_at: '2026-08-28T08:05:00Z',
        updater_status: {
          status: 'running',
          tag: 'zeuszs-v0.5.0',
          step: 'downloading',
          started_at: '2026-08-28T08:05:00Z',
          finished_at: null,
        },
      })
    )

    render(<PlatformUpdatePanel />, { wrapper: queryWrapper() })

    await user.click(await screen.findByRole('button', { name: 'Update now' }))
    await user.click(screen.getByRole('button', { name: 'Start update' }))

    expect(
      await screen.findByText('A platform update is available')
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Update now' })).toBeEnabled()
    expect(screen.queryByText('Platform update in progress')).toBeNull()
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      'Platform update completed: zeuszs-v0.5.0'
    )
  })

  test('shows a retryable inline failure when installation cannot start', async () => {
    const user = userEvent.setup()
    vi.mocked(getPlatformUpdate).mockResolvedValue(response(updateData()))
    vi.mocked(triggerPlatformUpdate).mockRejectedValue(
      new Error('host updater rejected request')
    )

    render(<PlatformUpdatePanel />, { wrapper: queryWrapper() })

    await user.click(await screen.findByRole('button', { name: 'Update now' }))
    await user.click(screen.getByRole('button', { name: 'Start update' }))

    expect(await screen.findByText('Platform update failed')).toBeVisible()
    expect(screen.getByText('host updater rejected request')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Update now' })).toBeEnabled()
  })
})
