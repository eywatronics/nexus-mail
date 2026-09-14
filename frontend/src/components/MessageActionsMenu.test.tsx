import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MessageActionsMenu } from './MessageActionsMenu'

const saveMessageAsEML = vi.fn()

vi.mock('../lib/api', () => ({
  saveMessageAsEML: (id: number) => saveMessageAsEML(id),
}))

beforeEach(() => {
  saveMessageAsEML.mockReset()
  saveMessageAsEML.mockResolvedValue(undefined)
})

const renderMenu = (showingSource = false, onToggleSource = () => {}) =>
  render(
    <MessageActionsMenu
      messageId={7}
      showingSource={showingSource}
      onToggleSource={onToggleSource}
    />,
  )

describe('message actions menu', () => {
  // The toolbar sits next to the subject line. Six buttons would push the
  // subject off the pane for controls almost nobody presses on a normal day.
  it('keeps its contents hidden until opened', () => {
    const { queryByTestId, getByTestId } = renderMenu()

    expect(queryByTestId('message-actions-menu')).toBeNull()
    fireEvent.click(getByTestId('open-message-actions'))
    expect(queryByTestId('message-actions-menu')).toBeTruthy()
  })

  it('closes when something outside it is clicked', async () => {
    const { getByTestId, queryByTestId } = renderMenu()
    fireEvent.click(getByTestId('open-message-actions'))

    document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))

    await waitFor(() => expect(queryByTestId('message-actions-menu')).toBeNull())
  })

  it('closes on Escape', async () => {
    const { getByTestId, queryByTestId } = renderMenu()
    fireEvent.click(getByTestId('open-message-actions'))

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    await waitFor(() => expect(queryByTestId('message-actions-menu')).toBeNull())
  })

  it('hands the source toggle back to the reading pane', () => {
    const onToggle = vi.fn()
    const { getByTestId } = renderMenu(false, onToggle)

    fireEvent.click(getByTestId('open-message-actions'))
    fireEvent.click(getByTestId('toggle-source'))

    expect(onToggle).toHaveBeenCalledOnce()
  })

  // The label has to say what pressing it does, not what is on screen. "View
  // source" while the source is already showing would be a dead end.
  it('offers the way back while the source is showing', () => {
    const { getByTestId } = renderMenu(true)

    fireEvent.click(getByTestId('open-message-actions'))
    expect(getByTestId('toggle-source').textContent).toContain('Show message')
  })

  it('saves the message and says so', async () => {
    const { getByTestId } = renderMenu()

    fireEvent.click(getByTestId('open-message-actions'))
    fireEvent.click(getByTestId('save-eml'))

    expect(saveMessageAsEML).toHaveBeenCalledWith(7)

    // The menu stays open: a save that closes it and opens a file manager
    // behind the window looks like nothing happened at all.
    await waitFor(() => {
      expect(getByTestId('save-eml').textContent).toContain('Saved')
    })
  })

  it('does not claim a failed save succeeded', async () => {
    saveMessageAsEML.mockRejectedValue(new Error('disk full'))
    const { getByTestId } = renderMenu()

    fireEvent.click(getByTestId('open-message-actions'))
    fireEvent.click(getByTestId('save-eml'))

    await waitFor(() => expect(saveMessageAsEML).toHaveBeenCalled())
    expect(getByTestId('save-eml').textContent).not.toContain('Saved')
  })
})
