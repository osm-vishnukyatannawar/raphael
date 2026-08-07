import { Check, ChevronsUpDown } from 'lucide-react'
import { useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

export type Option = {
  value: string
  label: string
  hint?: string
}

type Props = {
  options: Option[]
  selected: string[]
  onChange: (next: string[]) => void
  placeholder: string
  emptyMessage: string
  disabled?: boolean
}

/**
 * A filterable checkbox list in a popover.
 *
 * Deliberately not built on cmdk: the pickers here need nothing beyond
 * substring filtering and multi-select, and cmdk's own component wanted to
 * overwrite our vendored button.
 */
export default function MultiSelect({
  options,
  selected,
  onChange,
  placeholder,
  emptyMessage,
  disabled,
}: Props) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')

  const chosen = useMemo(() => new Set(selected), [selected])

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return options

    return options.filter(
      (o) =>
        o.label.toLowerCase().includes(q) ||
        (o.hint?.toLowerCase().includes(q) ?? false)
    )
  }, [options, query])

  function toggle(value: string) {
    const next = new Set(chosen)
    if (next.has(value)) {
      next.delete(value)
    } else {
      next.add(value)
    }
    onChange([...next])
  }

  /*
    Bulk actions apply to what is *visible*, not to every option. With no filter
    typed the two are the same, so "Select all" reads literally; with one typed
    it becomes "select everything matching", which is the only way to bulk-pick a
    subset out of eighty projects. Selecting a filtered set must not silently
    discard choices made under a different filter, hence the union rather than a
    replacement.
  */
  function selectVisible() {
    const next = new Set(chosen)
    for (const option of visible) next.add(option.value)
    onChange([...next])
  }

  function clearVisible() {
    const next = new Set(chosen)
    for (const option of visible) next.delete(option.value)
    onChange([...next])
  }

  const filtering = query.trim() !== ''
  const allVisibleChosen =
    visible.length > 0 && visible.every((o) => chosen.has(o.value))

  const label =
    selected.length === 0
      ? placeholder
      : selected.length === 1
        ? (options.find((o) => o.value === selected[0])?.label ?? placeholder)
        : `${selected.length} selected`

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          className="w-full justify-between font-normal"
        >
          <span
            className={selected.length === 0 ? 'text-muted-foreground' : ''}
          >
            {label}
          </span>
          <ChevronsUpDown className="size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>

      <PopoverContent
        className="w-(--radix-popover-trigger-width) p-0"
        align="start"
      >
        <div className="border-b p-2">
          <Input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Filter…"
            className="h-8"
          />
        </div>

        <ul className="max-h-64 overflow-y-auto p-1">
          {visible.length === 0 ? (
            <li className="text-muted-foreground p-3 text-center text-sm">
              {emptyMessage}
            </li>
          ) : (
            visible.map((option) => {
              const isSelected = chosen.has(option.value)

              return (
                <li key={option.value}>
                  <button
                    type="button"
                    onClick={() => toggle(option.value)}
                    className="hover:bg-accent flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm"
                  >
                    <span
                      className={`flex size-4 shrink-0 items-center justify-center rounded border ${
                        isSelected
                          ? 'bg-primary border-primary text-primary-foreground'
                          : 'border-input'
                      }`}
                    >
                      {isSelected && <Check className="size-3" />}
                    </span>
                    <span className="min-w-0 flex-1 truncate">
                      {option.label}
                    </span>
                    {option.hint && (
                      <span className="text-muted-foreground shrink-0 font-mono text-xs">
                        {option.hint}
                      </span>
                    )}
                  </button>
                </li>
              )
            })
          )}
        </ul>

        {options.length > 0 && (
          <div className="flex items-center justify-between gap-2 border-t px-2 py-1.5">
            <span className="text-muted-foreground text-xs">
              {selected.length} selected
            </span>
            <div className="flex gap-1">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-xs"
                disabled={visible.length === 0 || allVisibleChosen}
                onClick={selectVisible}
              >
                {filtering ? `Select ${visible.length} matching` : 'Select all'}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-xs"
                disabled={selected.length === 0}
                onClick={clearVisible}
              >
                {filtering ? 'Clear matching' : 'Clear'}
              </Button>
            </div>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
