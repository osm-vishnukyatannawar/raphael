import { useState } from 'react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SetDisplayName, SignIn } from '@wails/go/main/App'
import { type identity } from '@wails/go/models'

type Props = {
  onComplete: (session: identity.Session) => void
}

/**
 * Two-step first run: authenticate against Pinestem, then confirm the name
 * Raphael should use. The login comes first so step two can prefill the name
 * from the Pinestem account instead of making the user type it.
 */
export default function Onboarding({ onComplete }: Props) {
  const [session, setSession] = useState<identity.Session | null>(null)

  if (!session) {
    return <SignInStep onSignedIn={setSession} />
  }

  return <NameStep session={session} onComplete={onComplete} />
}

function SignInStep({
  onSignedIn,
}: {
  onSignedIn: (s: identity.Session) => void
}) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const canSubmit = email.trim() !== '' && password !== '' && !busy

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!canSubmit) return

    setBusy(true)
    setError(null)
    try {
      const result = await SignIn(email, password)
      if (result.invalidLogin) {
        setError('That email and password combination was not accepted.')
        return
      }
      if (result.errorMessage || !result.session) {
        setError(result.errorMessage || 'Sign in failed.')
        return
      }
      onSignedIn(result.session)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Shell step={1}>
      {/*
        Card lays its own children out with gap-(--card-spacing). Wrapping the
        sections in a <form> makes the card see a single child, collapsing that
        gap — so the form has to carry the layout itself.
      */}
      <form
        className="flex flex-col gap-(--card-spacing)"
        onSubmit={(e) => void submit(e)}
      >
        <CardHeader>
          <CardTitle>Connect Pinestem</CardTitle>
          <CardDescription>
            Raphael uses your Pinestem account to read your work. Credentials
            are sent straight to Pinestem and stored in your OS keyring.
          </CardDescription>
        </CardHeader>

        <CardContent className="space-y-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              type="email"
              autoComplete="username"
              autoFocus
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@osmosys.co"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
        </CardContent>

        <CardFooter>
          <Button type="submit" className="w-full" disabled={!canSubmit}>
            {busy ? 'Signing in…' : 'Sign in'}
          </Button>
        </CardFooter>
      </form>
    </Shell>
  )
}

function NameStep({
  session,
  onComplete,
}: {
  session: identity.Session
  onComplete: (s: identity.Session) => void
}) {
  // Prefilled from the Pinestem account — usually the user just shortens it.
  const [name, setName] = useState(session.displayName)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const canSubmit = name.trim() !== '' && !busy

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!canSubmit) return

    setBusy(true)
    setError(null)
    try {
      await SetDisplayName(name)
      onComplete({ ...session, displayName: name.trim() })
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <Shell step={2}>
      {/*
        Card lays its own children out with gap-(--card-spacing). Wrapping the
        sections in a <form> makes the card see a single child, collapsing that
        gap — so the form has to carry the layout itself.
      */}
      <form
        className="flex flex-col gap-(--card-spacing)"
        onSubmit={(e) => void submit(e)}
      >
        <CardHeader>
          <CardTitle>What should I call you?</CardTitle>
          <CardDescription>
            Signed in as {session.userName} at {session.companyName}.
          </CardDescription>
        </CardHeader>

        <CardContent className="space-y-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              onFocus={(e) => e.target.select()}
            />
            <p className="text-muted-foreground text-xs">
              Shorten it if you like — this is only how Raphael addresses you.
            </p>
          </div>

          {!session.secretsInKeyring && (
            <Alert>
              <AlertDescription>
                No OS keyring was available, so your session token is stored in
                Raphael&apos;s local database and your password was not saved.
              </AlertDescription>
            </Alert>
          )}
        </CardContent>

        <CardFooter>
          <Button type="submit" className="w-full" disabled={!canSubmit}>
            {busy ? 'Saving…' : 'Continue'}
          </Button>
        </CardFooter>
      </form>
    </Shell>
  )
}

function Shell({ step, children }: { step: 1 | 2; children: React.ReactNode }) {
  return (
    <main className="flex h-full items-center justify-center p-8">
      <div className="w-full max-w-md space-y-3">
        <p className="text-muted-foreground text-center text-xs tracking-wide uppercase">
          Step {step} of 2
        </p>
        <Card>{children}</Card>
      </div>
    </main>
  )
}
