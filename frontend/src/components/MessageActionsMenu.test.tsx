import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MessageActionsMenu } from './MessageActionsMenu'

const saveMessageAsEML = vi.fn()
const repairCharsets = vi.fn()
const repairEncoding = vi.fn()

vi.mock('../lib/api', () => ({
  saveMessageAsEML: (id: number) => saveMessageAsEML(id),
  repairCharsets: () => repairCharsets(),
  repairEncoding: (id: number, charset: string) => repairEncoding(id, charset),
}))

beforeEach(() => {
  saveMessageAsEML.mockReset()
  saveMessageAsEML.mockResolvedValue(undefined)
  repairCharsets.mockReset()
  repairCharsets.mockResolvedValue([
    { name: 'utf-8', label: 'Unicode (UTF-8)' },
    { name: 'iso-8859-9', label: 'Turkish (ISO-8859-9)' },
  ])
  repairEncoding.mockReset()
  repairEncoding.mockResolvedValue(undefined)
})

const renderMenu = (
  showingSource = false,
  onToggleSource = () => {},
  onRepaired = () => {},
) =>
  render(
    <MessageActionsMenu
      messageId={7}
      showingSource={showingSource}
      onToggleSource={onToggleSource}
      onRepaired={onRepaired}
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

describe('encoding repair', () => {
  // The reader who needs this is looking at mojibake. They know the message is
  // Turkish; they do not know it is Latin-5, so the list has to say the first.
  it('offers the encodings the backend supports, by language', async () => {
    const { getByTestId, findAllByTestId } = renderMenu()

    fireEvent.click(getByTestId('open-message-actions'))
    fireEvent.click(getByTestId('open-charset-picker'))

    const options = await findAllByTestId('charset-option')
    expect(options).toHaveLength(2)
    expect(options[1].textContent).toContain('Turkish')
  })

  // The list never changes, and fetching it for every message the reader
  // scrolls past would be a backend call per keystroke on a held arrow key.
  it('does not ask for the list until the menu is opened', async () => {
    const { getByTestId } = renderMenu()

    expect(repairCharsets).not.toHaveBeenCalled()

    fireEvent.click(getByTestId('open-message-actions'))
    await waitFor(() => expect(repairCharsets).toHaveBeenCalledOnce())
  })

  it('repairs with the chosen encoding and tells the pane to reload', async () => {
    const onRepaired = vi.fn()
    const { getByTestId, findAllByTestId } = renderMenu(false, () => {}, onRepaired)

    fireEvent.click(getByTestId('open-message-actions'))
    fireEvent.click(getByTestId('open-charset-picker'))
    const options = await findAllByTestId('charset-option')
    fireEvent.click(options[1])

    expect(repairEncoding).toHaveBeenCalledWith(7, 'iso-8859-9')
    await waitFor(() => expect(onRepaired).toHaveBeenCalledOnce())
  })

  // A repair that failed must not make the pane re-request a body that did not
  // change: the reader would see the same mojibake and think it had worked.
  it('does not reload the pane when the repair fails', async () => {
    repairEncoding.mockRejectedValue(new Error('unknown charset'))
    const onRepaired = vi.fn()
    const { getByTestId, findAllByTestId } = renderMenu(false, () => {}, onRepaired)

    fireEvent.click(getByTestId('open-message-actions'))
    fireEvent.click(getByTestId('open-charset-picker'))
    fireEvent.click((await findAllByTestId('charset-option'))[0])

    await waitFor(() => expect(repairEncoding).toHaveBeenCalled())
    expect(onRepaired).not.toHaveBeenCalled()
  })

  it('can back out of the picker without repairing anything', async () => {
    const { getByTestId, queryByTestId, findByTestId } = renderMenu()

    fireEvent.click(getByTestId('open-message-actions'))
    fireEvent.click(getByTestId('open-charset-picker'))
    fireEvent.click(await findByTestId('charset-back'))

    expect(queryByTestId('charset-option')).toBeNull()
    expect(getByTestId('save-eml')).toBeTruthy()
    expect(repairEncoding).not.toHaveBeenCalled()
  })
})
