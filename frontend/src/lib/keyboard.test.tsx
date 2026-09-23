import { fireEvent, render } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { useMessageShortcuts } from './keyboard'
import { useMailStore } from '../store/useMailStore'

/** A component that does nothing but install the global key handler. */
function Shortcuts() {
  useMessageShortcuts()
  return null
}

beforeEach(() => {
  useMailStore.getState().reset()
})

describe('find', () => {
  function pressCtrlF() {
    fireEvent.keyDown(window, { key: 'f', ctrlKey: true })
  }

  // The handler drops every modified keystroke, so a shortcut that wants one
  // has to sit above that guard. This is exactly how Ctrl+Z was unreachable
  // once before.
  it('opens the find bar on Ctrl+F', () => {
    useMailStore.setState({ selectedMessageId: 5 })
    render(<Shortcuts />)

    pressCtrlF()

    expect(useMailStore.getState().findOpen).toBe(true)
  })

  // Otherwise it would put a search box on screen with nothing behind it.
  it('does nothing with no message open', () => {
    render(<Shortcuts />)

    pressCtrlF()

    expect(useMailStore.getState().findOpen).toBe(false)
  })

  // Escape has to reach the bar from the message list too, not only from
  // inside the box.
  it('closes the find bar on Escape from anywhere', () => {
    useMailStore.setState({ selectedMessageId: 5, findOpen: true })
    render(<Shortcuts />)

    fireEvent.keyDown(window, { key: 'Escape' })

    expect(useMailStore.getState().findOpen).toBe(false)
  })

  // The find bar is the nearer of the two, so Escape shuts it first and leaves
  // the search results in the list alone.
  it('leaves the search alone while the find bar is open', () => {
    useMailStore.setState({
      selectedMessageId: 5,
      findOpen: true,
      searching: true,
      searchQuery: 'fatura',
    })
    render(<Shortcuts />)

    fireEvent.keyDown(window, { key: 'Escape' })

    expect(useMailStore.getState().findOpen).toBe(false)
    expect(useMailStore.getState().searching).toBe(true)
  })
})

describe('undo and redo', () => {
  const offer = { kind: 'trash' as const, count: 1, expiresUnixMs: Date.now() + 5000 }

  // Ctrl+Shift+Z has to be read as redo rather than as an undo with the shift
  // key held, which is what the undo branch would make of it if it looked
  // first. Both handlers clear their own offer before calling the backend, so
  // which offer is gone says which branch ran.
  it('reads Ctrl+Shift+Z as redo, not as a shifted undo', () => {
    useMailStore.setState({ undoOffer: offer, redoOffer: offer })
    render(<Shortcuts />)

    fireEvent.keyDown(window, { key: 'Z', ctrlKey: true, shiftKey: true })

    expect(useMailStore.getState().redoOffer).toBeNull()
    expect(useMailStore.getState().undoOffer).not.toBeNull()
  })

  // The platforms disagree about which key means redo, and people carry the
  // habit of whichever they learned first.
  it('reads Ctrl+Y as redo too', () => {
    useMailStore.setState({ redoOffer: offer })
    render(<Shortcuts />)

    fireEvent.keyDown(window, { key: 'y', ctrlKey: true })

    expect(useMailStore.getState().redoOffer).toBeNull()
  })

  it('still reads a plain Ctrl+Z as undo', () => {
    useMailStore.setState({ undoOffer: offer, redoOffer: offer })
    render(<Shortcuts />)

    fireEvent.keyDown(window, { key: 'z', ctrlKey: true })

    expect(useMailStore.getState().undoOffer).toBeNull()
    expect(useMailStore.getState().redoOffer).not.toBeNull()
  })
})
