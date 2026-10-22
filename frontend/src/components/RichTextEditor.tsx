import {
  LinkSimple,
  ListBullets,
  ListNumbers,
  TextB,
  TextItalic,
  TextUnderline,
  X,
} from '@phosphor-icons/react'
import { useEffect, useRef, useState } from 'react'
import {
  escapeHTML,
  htmlToText,
  normalizeLinkHref,
  sanitizeComposeHTML,
  textToHTML,
} from '../lib/richText'
import {
  BUTTON_GHOST,
  BUTTON_PRIMARY,
  ICON,
  ICON_ONLY,
  INPUT,
  RADIUS,
  SURFACE,
  TEXT,
} from '../lib/ui'

interface RichTextEditorProps {
  /** Sanitised before it reaches the editable region. */
  initialHTML: string
  onChange: (html: string, text: string) => void
  autoFocus?: boolean
  /** Switches the composer back to a plain textarea. */
  onPlain?: () => void
}

/**
 * A small contenteditable editor.
 *
 * `execCommand` is deprecated, not absent: every WebView this app ships on
 * still implements it, and the alternative is a hand-written range engine for
 * bold, lists and links. The commands used here are the four a mail composer
 * actually needs, and the output is sanitised before it leaves the component.
 */
export function RichTextEditor({
  initialHTML,
  onChange,
  autoFocus = false,
  onPlain,
}: RichTextEditorProps) {
  const editor = useRef<HTMLDivElement>(null)
  const savedRange = useRef<Range | null>(null)

  const [empty, setEmpty] = useState(initialHTML.trim() === '')
  const [linkOpen, setLinkOpen] = useState(false)
  const [linkValue, setLinkValue] = useState('')
  const [linkError, setLinkError] = useState('')

  useEffect(() => {
    const element = editor.current
    if (!element) return
    element.innerHTML = sanitizeComposeHTML(initialHTML)
    setEmpty((element.textContent ?? '').trim() === '')

    if (autoFocus) {
      element.focus()
      const range = document.createRange()
      range.selectNodeContents(element)
      range.collapse(false)
      const selection = window.getSelection()
      selection?.removeAllRanges()
      selection?.addRange(range)
    }
    // initialHTML is the opening value; the editor owns the DOM after this.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const emit = () => {
    const element = editor.current
    if (!element) return
    const html = sanitizeComposeHTML(element.innerHTML)
    setEmpty((element.textContent ?? '').trim() === '')
    onChange(html, htmlToText(html))
  }

  const run = (command: string, value?: string) => {
    editor.current?.focus()
    document.execCommand(command, false, value)
    emit()
  }

  const openLink = () => {
    const selection = window.getSelection()
    savedRange.current =
      selection && selection.rangeCount > 0 ? selection.getRangeAt(0).cloneRange() : null
    setLinkValue('')
    setLinkError('')
    setLinkOpen(true)
  }

  const applyLink = () => {
    const href = normalizeLinkHref(linkValue)
    if (href === null) {
      setLinkError('Use an http(s) or mailto address.')
      return
    }

    const element = editor.current
    if (!element) return
    element.focus()

    const selection = window.getSelection()
    if (savedRange.current && selection) {
      selection.removeAllRanges()
      selection.addRange(savedRange.current)
    }

    const collapsed = !selection || selection.isCollapsed
    if (collapsed) {
      document.execCommand(
        'insertHTML',
        false,
        `<a href="${escapeHTML(href)}">${escapeHTML(href)}</a>`,
      )
    } else {
      document.execCommand('createLink', false, href)
    }

    setLinkOpen(false)
    setLinkValue('')
    setLinkError('')
    emit()
  }

  const paste = (event: React.ClipboardEvent<HTMLDivElement>) => {
    const html = event.clipboardData.getData('text/html')
    const text = event.clipboardData.getData('text/plain')
    if (html === '' && text === '') return

    event.preventDefault()
    const insert = html !== '' ? sanitizeComposeHTML(html) : textToHTML(text)
    document.execCommand('insertHTML', false, insert)
    emit()
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div
        className={`relative flex flex-wrap items-center gap-0.5 border-b px-6 py-1 ${SURFACE.divider}`}
      >
        <ToolbarButton label="Bold" testId="rich-bold" onClick={() => run('bold')}>
          <TextB size={ICON.size} weight="bold" aria-hidden />
        </ToolbarButton>
        <ToolbarButton label="Italic" testId="rich-italic" onClick={() => run('italic')}>
          <TextItalic size={ICON.size} weight={ICON.weight} aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          label="Underline"
          testId="rich-underline"
          onClick={() => run('underline')}
        >
          <TextUnderline size={ICON.size} weight={ICON.weight} aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          label="Bulleted list"
          testId="rich-bullets"
          onClick={() => run('insertUnorderedList')}
        >
          <ListBullets size={ICON.size} weight={ICON.weight} aria-hidden />
        </ToolbarButton>
        <ToolbarButton
          label="Numbered list"
          testId="rich-numbers"
          onClick={() => run('insertOrderedList')}
        >
          <ListNumbers size={ICON.size} weight={ICON.weight} aria-hidden />
        </ToolbarButton>
        <ToolbarButton label="Link" testId="rich-link" onClick={openLink}>
          <LinkSimple size={ICON.size} weight={ICON.weight} aria-hidden />
        </ToolbarButton>

        {onPlain && (
          <button
            type="button"
            data-testid="composer-format"
            onClick={onPlain}
            className={`${BUTTON_GHOST} ml-auto text-xs`}
          >
            Plain text
          </button>
        )}

        {linkOpen && (
          <div
            className={`absolute left-6 top-full z-20 mt-1 w-80 border p-2 shadow-lg ${RADIUS} ${SURFACE.divider} ${SURFACE.page}`}
          >
            <label htmlFor="rich-link-url" className={`text-xs font-medium ${TEXT.secondary}`}>
              Link address
            </label>
            <div className="mt-1 flex items-center gap-2">
              <input
                id="rich-link-url"
                data-testid="rich-link-url"
                autoFocus
                value={linkValue}
                onChange={(event) => {
                  setLinkValue(event.target.value)
                  setLinkError('')
                }}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    applyLink()
                  }
                  if (event.key === 'Escape') setLinkOpen(false)
                }}
                placeholder="https://example.com"
                className={`${INPUT} min-w-0 flex-1 py-1`}
              />
              <button
                type="button"
                data-testid="rich-link-apply"
                onClick={applyLink}
                className={`${BUTTON_PRIMARY} py-1 text-xs`}
              >
                Add
              </button>
              <button
                type="button"
                aria-label="Cancel link"
                onClick={() => setLinkOpen(false)}
                className={`${BUTTON_GHOST} ${ICON_ONLY}`}
              >
                <X size={ICON.size} weight={ICON.weight} aria-hidden />
              </button>
            </div>
            {linkError && (
              <p className={`mt-1 text-xs ${TEXT.secondary}`} role="alert">
                {linkError}
              </p>
            )}
          </div>
        )}
      </div>

      <div className="relative flex min-h-0 flex-1 flex-col">
        {empty && (
          <span
            aria-hidden
            className={`pointer-events-none absolute left-6 top-4 text-sm ${TEXT.muted}`}
          >
            Write a message
          </span>
        )}
        <div
          ref={editor}
          data-testid="rich-editor"
          contentEditable
          suppressContentEditableWarning
          role="textbox"
          aria-multiline="true"
          aria-label="Message"
          onInput={emit}
          onPaste={paste}
          className={`rich-editor min-h-0 flex-1 overflow-y-auto px-6 py-4 text-sm leading-relaxed ${TEXT.primary}`}
        />
      </div>
    </div>
  )
}

function ToolbarButton({
  label,
  testId,
  onClick,
  children,
}: {
  label: string
  testId: string
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      data-testid={testId}
      // Keeping the focus in the editor is what makes the command apply to
      // the selection the writer made rather than to a collapsed caret.
      onMouseDown={(event) => event.preventDefault()}
      onClick={onClick}
      className={`${BUTTON_GHOST} ${ICON_ONLY}`}
    >
      {children}
    </button>
  )
}
