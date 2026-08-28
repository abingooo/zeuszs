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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { updateSystemOption } from '../../api'
import { SettingsPageProvider } from '../../components/settings-page-context'
import { IDVisibilitySection } from '../id-visibility-section'

vi.mock('../../api', () => ({
  updateSystemOption: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

function IDVisibilitySectionHarness(props: {
  defaultValue: boolean
  queryClient: QueryClient
  actionsContainer: HTMLDivElement
}) {
  return (
    <QueryClientProvider client={props.queryClient}>
      <SettingsPageProvider
        actionsContainer={props.actionsContainer}
        suppressSectionHeader={false}
      >
        <IDVisibilitySection defaultValue={props.defaultValue} />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })
}

function createActionsContainer() {
  const actionsContainer = document.createElement('div')
  actionsContainer.dataset.idVisibilityActions = ''
  document.body.append(actionsContainer)
  return actionsContainer
}

describe('IDVisibilitySection', () => {
  beforeEach(() => {
    vi.mocked(updateSystemOption).mockResolvedValue({
      success: true,
      message: '',
    })
  })

  afterEach(() => {
    document
      .querySelectorAll('[data-id-visibility-actions]')
      .forEach((element) => element.remove())
  })

  test('shows the disabled default and explains which roles are affected', () => {
    const queryClient = createQueryClient()
    const actionsContainer = createActionsContainer()

    render(
      <IDVisibilitySectionHarness
        defaultValue={false}
        queryClient={queryClient}
        actionsContainer={actionsContainer}
      />
    )

    expect(
      screen.getByRole('switch', { name: 'ID Visibility' })
    ).not.toBeChecked()
    expect(
      screen.getByText(
        'Allow non-platform users to view organization and user IDs. Platform administrators and platform owners can always view them.'
      )
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()

    queryClient.clear()
  })

  test('resets the switch when the saved default changes', async () => {
    const queryClient = createQueryClient()
    const actionsContainer = createActionsContainer()
    const view = render(
      <IDVisibilitySectionHarness
        defaultValue={false}
        queryClient={queryClient}
        actionsContainer={actionsContainer}
      />
    )

    view.rerender(
      <IDVisibilitySectionHarness
        defaultValue
        queryClient={queryClient}
        actionsContainer={actionsContainer}
      />
    )

    await waitFor(() => {
      expect(
        screen.getByRole('switch', { name: 'ID Visibility' })
      ).toBeChecked()
    })
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()

    queryClient.clear()
  })

  test('saves the selected visibility value under the custom option key', async () => {
    const user = userEvent.setup()
    const queryClient = createQueryClient()
    const actionsContainer = createActionsContainer()

    render(
      <IDVisibilitySectionHarness
        defaultValue={false}
        queryClient={queryClient}
        actionsContainer={actionsContainer}
      />
    )

    const visibilitySwitch = screen.getByRole('switch', {
      name: 'ID Visibility',
    })
    const saveButton = screen.getByRole('button', { name: 'Save Changes' })

    await user.click(visibilitySwitch)

    expect(visibilitySwitch).toBeChecked()
    expect(saveButton).toBeEnabled()

    await user.click(saveButton)

    await waitFor(() => {
      expect(updateSystemOption).toHaveBeenCalledWith({
        key: 'custom_setting.id_visibility_enabled',
        value: true,
      })
    })
    await waitFor(() => {
      expect(saveButton).toBeDisabled()
    })

    queryClient.clear()
  })

  test('keeps the changed value retryable when the server rejects the update', async () => {
    vi.mocked(updateSystemOption).mockResolvedValue({
      success: false,
      message: 'update rejected',
    })
    const user = userEvent.setup()
    const queryClient = createQueryClient()
    const actionsContainer = createActionsContainer()

    render(
      <IDVisibilitySectionHarness
        defaultValue={false}
        queryClient={queryClient}
        actionsContainer={actionsContainer}
      />
    )

    const visibilitySwitch = screen.getByRole('switch', {
      name: 'ID Visibility',
    })
    const saveButton = screen.getByRole('button', { name: 'Save Changes' })

    await user.click(visibilitySwitch)
    await user.click(saveButton)

    await waitFor(() => {
      expect(updateSystemOption).toHaveBeenCalledWith({
        key: 'custom_setting.id_visibility_enabled',
        value: true,
      })
    })
    expect(visibilitySwitch).toBeChecked()
    expect(saveButton).toBeEnabled()

    queryClient.clear()
  })
})
