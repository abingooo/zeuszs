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
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { UserProfile } from '../../types'
import { ProfileHeader } from '../profile-header'

const profile: UserProfile = {
  id: 42,
  username: 'profile-user',
  display_name: 'Profile User',
  role: ROLE.USER,
  email: 'profile@example.com',
  group: 'default',
  quota: 10_000,
  used_quota: 2_000,
  request_count: 12,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 1_700_000_000,
}

function setIDVisibility(enabled: boolean) {
  useAuthStore.getState().auth.setUser({
    id: profile.id,
    username: profile.username,
    role: ROLE.USER,
    permissions: { id_visible: enabled },
  })
}

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
})

describe('ProfileHeader ID visibility', () => {
  test('hides the user ID when visibility is disabled', () => {
    setIDVisibility(false)

    render(<ProfileHeader profile={profile} loading={false} />)

    expect(screen.queryByText('User ID 42')).not.toBeInTheDocument()
  })

  test('shows the user ID when visibility is enabled', () => {
    setIDVisibility(true)

    render(<ProfileHeader profile={profile} loading={false} />)

    expect(screen.getByText('User ID 42')).toBeVisible()
  })
})
