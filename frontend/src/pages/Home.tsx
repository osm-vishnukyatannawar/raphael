import { Building2, LogOut, Settings as SettingsIcon } from 'lucide-react'
import { useEffect, useState } from 'react'

import SettingsDialog from '@/components/SettingsDialog'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import Monitors from '@/pages/Monitors'
import Tasks from '@/pages/Tasks'
import { GetSettings, SignOut } from '@wails/go/main/App'
import { type identity, type settings } from '@wails/go/models'

type Props = {
  session: identity.Session
  onSignedOut: () => void
}

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

/**
 * The signed-in shell: identity, settings, and the tab switch.
 *
 * Settings live here rather than in a tab because they apply to both.
 */
export default function Home({ session, onSignedOut }: Props) {
  const [prefs, setPrefs] = useState<settings.Settings | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)

  useEffect(() => {
    let cancelled = false

    void (async () => {
      const loaded = await GetSettings()
      if (!cancelled) setPrefs(loaded)
    })()

    return () => {
      cancelled = true
    }
  }, [])

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
            <p className="text-sm font-medium">
              {greeting()}, {session.displayName.split(' ')[0]}
            </p>
            <p className="text-muted-foreground flex items-center gap-1 text-xs">
              <Building2 className="size-3" />
              {session.companyName}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            title="Settings"
            onClick={() => setSettingsOpen(true)}
          >
            <SettingsIcon className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void SignOut().then(onSignedOut)}
          >
            <LogOut className="size-4" />
            Sign out
          </Button>
        </div>
      </header>

      {/*
        Both tabs stay mounted (forceMount) so switching away doesn't tear down
        their event subscriptions and make them re-fetch on every switch.
      */}
      <Tabs
        defaultValue="review"
        className="flex min-h-0 flex-1 flex-col gap-0"
      >
        <div className="flex justify-center border-b px-6 py-2">
          <TabsList>
            <TabsTrigger value="review">Review</TabsTrigger>
            <TabsTrigger value="targets">Targets</TabsTrigger>
          </TabsList>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
          <TabsContent
            value="review"
            forceMount
            className="data-[state=inactive]:hidden"
          >
            <Tasks prefs={prefs} />
          </TabsContent>
          <TabsContent
            value="targets"
            forceMount
            className="data-[state=inactive]:hidden"
          >
            <Monitors />
          </TabsContent>
        </div>
      </Tabs>

      {prefs && (
        <SettingsDialog
          open={settingsOpen}
          onOpenChange={setSettingsOpen}
          current={prefs}
          onSaved={setPrefs}
        />
      )}
    </div>
  )
}
