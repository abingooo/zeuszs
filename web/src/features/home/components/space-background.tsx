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
export function SpaceBackground() {
  return (
    <div
      data-home-space-background
      aria-hidden='true'
      className='pointer-events-none absolute inset-0 overflow-hidden'
    >
      <span className='home-space-stars home-space-stars-near absolute top-0 left-0 block rounded-full text-slate-600/25 dark:text-slate-100/40' />
      <span className='home-space-stars home-space-stars-far absolute top-0 left-0 block rounded-full text-sky-700/20 dark:text-sky-100/30' />
    </div>
  )
}
