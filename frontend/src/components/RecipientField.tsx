import { X } from '@phosphor-icons/react'
import { useEffect, useRef, useState } from 'react'
import { suggestRecipients, type Recipient } from '../lib/api'
import {
  addressIn,
  formatRecipient,
  formatRecipientLine,
  pillLabel,
  splitRecipientLine,
} from '../lib/recipients'
import { BUTTON_GHOST, ICON, RADIUS, SURFACE, TEXT } from '../lib/ui'

interface RecipientFieldProps {
  id: string
  accountId: number
  /** The address line as the backend will receive it. */
  value: string
  onChange: (next: string) => void
  /** Puts the cursor here when a new message opens. */
  autoFocus?: boolean
}

/**
 * An address line that shows committed addresses as pills and completes what
 * is being typed from people who have already written to this account.
 *
 * The pills are a view of the line, not a second address model: each one keeps
 * the text the writer committed or the name a suggestion came with, and the
 * backend still does the only parsing that matters.
 */
export function RecipientField({
  id,
  accountId,
  value,
  onChange,
  autoFocus = false,
}: RecipientFieldProps) {
  const [pills, setPills] = useState<string[]>(() => {
    const parts = splitRecipientLine(value)
    return parts.slice(0, -1).filter(Boolean)
  })
  const [draft, setDraft] = useState(() => {
    const parts = splitRecipientLine(value)
    return parts[parts.length - 1] ?? ''
  })

  const [suggestions, setSuggestions] = useState<Recipient[]>([])
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(-1)
  const [focused, setFocused] = useState(false)

  const input = useRef<HTMLInputElement>(null)
  // What this field last handed to the parent, so the value coming back down
  // from its own onChange does not look like an edit and get emitted again.
  const emitted = useRef(value)

  // After mount the line belongs to this field. Composer opens it with a
  // draft's addresses or a reply's recipient, and every later change starts
  // here; a value prop changing underneath would mean a second writer to the
  // same line, which is the ambiguity pills exist to avoid.

  useEffect(() => {
    const next = formatRecipientLine(pills, draft)
    if (next === emitted.current) return
    emitted.current = next
    onChange(next)
  }, [pills, draft, onChange])

  useEffect(() => {
    if (autoFocus) input.current?.focus()
  }, [autoFocus])

  useEffect(() => {
    const query = draft.trim()
    if (!focused || query.length < 2) {
      setSuggestions([])
      setOpen(false)
      setActive(-1)
      return
    }

    let cancelled = false
    const timer = setTimeout(() => {
      suggestRecipients(accountId, query, 8)
        .then((list) => {
          if (cancelled) return
          const used = new Set(pills.map((p) => addressIn(p).toLowerCase()))
          const next = list.filter(
            (r) => r.addr !== '' && !used.has(r.addr.toLowerCase()),
          )
          setSuggestions(next)
          setOpen(next.length > 0)
          setActive(-1)
        })
        .catch(() => {
          // Completion is an offer, not a feature the message depends on. A
          // failed lookup should not put an error over the message being
          // written.
          if (!cancelled) {
            setSuggestions([])
            setOpen(false)
          }
        })
    }, 150)

    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [accountId, draft, focused, pills])

  const commit = (text = draft) => {
    const trimmed = text.trim()
    if (trimmed === '') return
    const key = addressIn(trimmed).toLowerCase()
    setPills((current) => {
      if (current.some((p) => addressIn(p).toLowerCase() === key)) return current
      return [...current, trimmed]
    })
    setDraft('')
    setOpen(false)
    setActive(-1)
  }

  const pick = (recipient: Recipient) => {
    commit(formatRecipient(recipient.name, recipient.addr))
    setSuggestions([])
    input.current?.focus()
  }

  const change = (raw: string) => {
    const parts = splitRecipientLine(raw)
    if (parts.length > 1) {
      const finished = parts.slice(0, -1).map((p) => p.trim()).filter(Boolean)
      if (finished.length > 0) {
        setPills((current) => {
          const have = new Set(current.map((p) => addressIn(p).toLowerCase()))
          const additions = finished.filter((p) => {
            const key = addressIn(p).toLowerCase()
            if (have.has(key)) return false
            have.add(key)
            return true
          })
          return additions.length > 0 ? [...current, ...additions] : current
        })
      }
      setDraft(parts[parts.length - 1] ?? '')
    } else {
      setDraft(raw)
    }
    setActive(-1)
    setOpen(true)
  }

  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown' && suggestions.length > 0) {
      event.preventDefault()
      setOpen(true)
      setActive((current) => (current + 1) % suggestions.length)
      return
    }
    if (event.key === 'ArrowUp' && suggestions.length > 0) {
      event.preventDefault()
      setOpen(true)
      setActive((current) => (current <= 0 ? suggestions.length - 1 : current - 1))
      return
    }
    if (event.key === 'Enter') {
      event.preventDefault()
      if (open && active >= 0 && suggestions[active]) {
        pick(suggestions[active])
        return
      }
      commit()
      return
    }
    if (event.key === 'Escape') {
      setOpen(false)
      setActive(-1)
      return
    }
    if (event.key === 'Backspace' && draft === '' && pills.length > 0) {
      event.preventDefault()
      setPills((current) => current.slice(0, -1))
      return
    }
    // Tabbing away completes the address instead of abandoning it.
    if (event.key === 'Tab' && draft.trim() !== '') commit()
  }

  const editPill = (index: number) => {
    const pill = pills[index]
    setPills((current) => current.filter((_, i) => i !== index))
    setDraft(pill)
    input.current?.focus()
  }

  const activeId = active >= 0 ? `${id}-option-${active}` : undefined

  return (
    <div
      className={`relative min-w-0 flex-1 ${RADIUS} focus-within:outline focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-[var(--color-accent)]`}
    >
      <div className="flex flex-wrap items-center gap-1 py-1">
        {pills.map((pill, index) => (
          <span
            key={`${pill}-${index}`}
            className={`inline-flex max-w-full items-center gap-1 border py-0.5 pl-2 pr-0.5 text-xs ${RADIUS} ${SURFACE.divider} ${TEXT.primary}`}
          >
            <button
              type="button"
              onClick={() => editPill(index)}
              title={`Edit ${pill}`}
              className="max-w-48 truncate text-left"
            >
              {pillLabel(pill)}
            </button>
            <button
              type="button"
              aria-label={`Remove ${pill}`}
              title={`Remove ${pill}`}
              onClick={() =>
                setPills((current) => current.filter((_, i) => i !== index))
              }
              className={`${BUTTON_GHOST} p-0.5`}
            >
              <X size={ICON.size} weight={ICON.weight} aria-hidden />
            </button>
          </span>
        ))}

        <input
          ref={input}
          id={id}
          data-testid={id}
          type="text"
          role="combobox"
          aria-expanded={open && suggestions.length > 0}
          aria-controls={`${id}-suggestions`}
          aria-autocomplete="list"
          aria-activedescendant={activeId}
          autoComplete="off"
          value={draft}
          onChange={(event) => change(event.target.value)}
          onKeyDown={onKeyDown}
          onFocus={() => setFocused(true)}
          onBlur={() => {
            commit()
            setFocused(false)
            setOpen(false)
            setActive(-1)
          }}
          placeholder={pills.length === 0 ? 'name@example.com' : ''}
          className={`min-w-24 flex-1 bg-transparent py-1 text-sm outline-none placeholder:text-neutral-500 focus-visible:outline-none dark:placeholder:text-neutral-400 ${TEXT.primary}`}
        />
      </div>

      {open && suggestions.length > 0 && (
        <ul
          id={`${id}-suggestions`}
          role="listbox"
          className={`absolute left-0 right-0 top-full z-20 mt-1 max-h-56 overflow-y-auto border p-1 shadow-lg ${RADIUS} ${SURFACE.divider} ${SURFACE.panel}`}
        >
          {suggestions.map((recipient, index) => (
            <li
              key={recipient.addr}
              id={`${id}-option-${index}`}
              role="option"
              aria-selected={index === active}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => pick(recipient)}
              className={`cursor-pointer px-2 py-1.5 ${index === active ? 'bg-neutral-100 dark:bg-neutral-800' : ''}`}
            >
              <span className={`block truncate text-sm ${TEXT.primary}`}>
                {recipient.name || recipient.addr}
              </span>
              {recipient.name !== '' && (
                <span className={`block truncate text-xs ${TEXT.muted}`}>
                  {recipient.addr}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
