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
      </PopoverContent>
    </Popover>
  )
}
