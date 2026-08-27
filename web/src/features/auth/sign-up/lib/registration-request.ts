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
import type { RegisterPayload } from '@/features/auth/types'

type RegistrationRequestInput = {
  username: string
  password: string
  email?: string
  verificationCode?: string
  affiliateCode?: string
  organizationInviteCode?: string
  turnstile?: string
}

export function createRegistrationRequest(
  input: RegistrationRequestInput
): RegisterPayload {
  return {
    username: input.username,
    password: input.password,
    email: input.email || undefined,
    verification_code: input.verificationCode || undefined,
    aff_code: input.affiliateCode || undefined,
    organization_invite_code: input.organizationInviteCode?.trim() || undefined,
    turnstile: input.turnstile,
  }
}
