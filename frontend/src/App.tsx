import { useCallback, useEffect, useState } from 'react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import Tasks from '@/pages/Tasks'
import Onboarding from '@/pages/Onboarding'
import { Bootstrap } from '@wails/go/main/App'
import { type identity } from '@wails/go/models'

type State =
  | { status: 'loading' }
  | { status: 'onboarding' }
  | { status: 'ready'; session: identity.Session }
  | { status: 'failed'; message: string }

export default function App() {
  const [state, setState] = useState<State>({ status: 'loading' })

  const bootstrap = useCallback(() => {
    Bootstrap()
      .then((result) => {
        if (result.error) {
          setState({ status: 'failed', message: result.error })
          return
        }
        setState(
          result.onboarded && result.session
            ? { status: 'ready', session: result.session }
            : { status: 'onboarding' }
        )
      })
      .catch((err: unknown) => {
        setState({
          status: 'failed',
          message: err instanceof Error ? err.message : String(err),
        })
      })
  }, [])

  useEffect(bootstrap, [bootstrap])

  switch (state.status) {
    case 'loading':
      return <BootSkeleton />

    case 'failed':
      return (
        <main className="flex h-full items-center justify-center p-8">
          <Alert variant="destructive" className="max-w-md">
            <AlertTitle>Raphael could not start</AlertTitle>
            <AlertDescription>{state.message}</AlertDescription>
          </Alert>
        </main>
      )

    case 'onboarding':
      return (
        <Onboarding
          onComplete={(session) => setState({ status: 'ready', session })}
        />
      )

    case 'ready':
      return (
        <Tasks
          session={state.session}
          onSignedOut={() => setState({ status: 'onboarding' })}
        />
      )
  }
}

function BootSkeleton() {
  return (
    <main className="flex h-full items-center justify-center p-8">
      <div className="w-full max-w-md space-y-3">
        <Skeleton className="h-6 w-40" />
        <Skeleton className="h-48 w-full" />
      </div>
    </main>
  )
}
