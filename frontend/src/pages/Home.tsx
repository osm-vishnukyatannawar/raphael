import { Building2, LogOut } from 'lucide-react'
import { useState } from 'react'

import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { SignOut } from '@wails/go/main/App'
import { type identity } from '@wails/go/models'

type Props = {
  session: identity.Session
  onSignedOut: () => void
}

/** Greeting that tracks the local clock rather than being fixed at "Welcome". */
function greeting(now = new Date()): string {
  const hour = now.getHours()
  if (hour < 12) return 'Good morning'
  if (hour < 18) return 'Good afternoon'
  return 'Good evening'
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  const first = parts[0][0]
  const last = parts.length > 1 ? parts[parts.length - 1][0] : ''

  return (first + last).toUpperCase()
}

export default function Home({ session, onSignedOut }: Props) {
  const [busy, setBusy] = useState(false)

  async function signOut() {
    setBusy(true)
    try {
      await SignOut()
      onSignedOut()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between border-b px-6 py-3">
        <div className="flex items-center gap-3">
          <Avatar className="size-8">
            <AvatarFallback className="text-xs">
              {initials(session.displayName)}
            </AvatarFallback>
          </Avatar>
          <div className="leading-tight">
            <p className="text-sm font-medium">{session.displayName}</p>
            <p className="text-muted-foreground text-xs">{session.userName}</p>
          </div>
        </div>

        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          onClick={() => void signOut()}
        >
          <LogOut className="size-4" />
          Sign out
        </Button>
      </header>

      <main className="flex flex-1 items-center justify-center p-8">
        <Card className="w-full max-w-lg">
          <CardHeader>
            <CardTitle className="text-2xl">
              {greeting()}, {session.displayName.split(' ')[0]}
            </CardTitle>
            <CardDescription>
              Raphael is connected. Features land here next.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="text-muted-foreground flex items-center gap-2 text-sm">
              <Building2 className="size-4" />
              <span>
                {session.companyName}
                <span className="text-muted-foreground/60">
                  {' '}
                  · company {session.companyId}
                </span>
              </span>
            </div>

            {!session.secretsInKeyring && (
              <p className="text-muted-foreground text-xs">
                Session token is stored locally — no OS keyring was available.
              </p>
            )}
          </CardContent>
        </Card>
      </main>
    </div>
  )
}
