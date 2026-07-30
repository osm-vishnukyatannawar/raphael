import { useCallback, useEffect, useRef, useState } from 'react'

import BillingBar from '@/components/BillingBar'
import MyTasks from '@/pages/MyTasks'
import Tasks from '@/pages/Tasks'
import { GetBilling, RefreshBilling } from '@wails/go/main/App'
import { type main, type settings } from '@wails/go/models'
import { EventsOff, EventsOn } from '@wails/runtime/runtime'

const EVENT_BILLING = 'billing:updated'

type Props = {
  prefs: settings.Settings | null
}

/**
 * The always-on screen: hours across the top, then both task lists side by side.
 *
 * Two columns rather than a tab each, because the point of the assigned list is
 * to be visible at the same time as the review queue. They are stacked below
 * 1280px so a smaller window degrades rather than squeezing both into unreadable
 * columns — the target display is 1920 wide.
 *
 * Billing lives here rather than inside either list: it belongs to neither, and
 * hoisting it means one subscription instead of one per column.
 */
export default function Dashboard({ prefs }: Props) {
  const [billing, setBilling] = useState<main.BillingResult | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  const inFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    setRefreshing(true)
    try {
      setBilling(await RefreshBilling())
    } finally {
      inFlight.current = false
      setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    void (async () => {
      const cached = await GetBilling()
      if (!cancelled) setBilling(cached)
    })()

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    EventsOn(EVENT_BILLING, (payload: main.BillingResult) =>
      setBilling(payload)
    )

    return () => {
      EventsOff(EVENT_BILLING)
    }
  }, [])

  return (
    <div className="w-full px-6 py-5">
      <BillingBar
        result={billing}
        refreshing={refreshing}
        onRefresh={() => void refresh()}
      />

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <Tasks prefs={prefs} />
        <MyTasks prefs={prefs} />
      </div>
    </div>
  )
}
