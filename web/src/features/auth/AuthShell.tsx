import type { ReactNode } from 'react'

import { Card } from '@kipit-labs/kipitidy/card'
import { LanguageSwitcher } from '@/features/account/LanguageSwitcher'
import { Logo } from '@/features/layout/Logo'

/** Centered, branded card used by the sign-in and email verification pages. */
export function AuthShell({ children }: { children: ReactNode }) {
  return (
    <div className="bg-muted/40 flex min-h-svh items-center justify-center p-4">
      <div className="w-full max-w-sm space-y-6">
        {/* A full page load: the home page is served by the server, not the app. */}
        <a href="/" className="flex justify-center">
          <Logo className="text-xl" />
        </a>
        <Card>{children}</Card>
        <div className="flex justify-center">
          <LanguageSwitcher />
        </div>
      </div>
    </div>
  )
}
