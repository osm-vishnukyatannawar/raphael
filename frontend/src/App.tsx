import { useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Info } from '@wails/go/main/App'
import { type main } from '@wails/go/models'

/**
 * Placeholder shell. It exists to keep the full pipeline exercised —
 * Go binding -> generated TypeScript model -> Tailwind -> shadcn component —
 * so a break anywhere in that chain shows up before real screens are built.
 */
export default function App() {
  const [info, setInfo] = useState<main.AppInfo | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    Info()
      .then(setInfo)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [])

  return (
    <main className="bg-background flex h-full items-center justify-center p-8">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Raphael</CardTitle>
          <CardDescription>
            Scaffold only — no features wired up yet.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {error ? (
            <p className="text-destructive text-sm">{error}</p>
          ) : (
            <dl className="text-muted-foreground grid grid-cols-2 gap-y-1 text-sm">
              <dt>Version</dt>
              <dd className="text-foreground font-mono">
                {info?.version ?? '…'}
              </dd>
              <dt>Platform</dt>
              <dd className="text-foreground font-mono">
                {info?.platform ?? '…'}
              </dd>
            </dl>
          )}
          <Button
            className="w-full"
            onClick={() => {
              void Info().then(setInfo)
            }}
          >
            Refresh
          </Button>
        </CardContent>
      </Card>
    </main>
  )
}
