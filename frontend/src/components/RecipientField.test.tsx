import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RecipientField } from './RecipientField'

const suggestRecipients = vi.fn()

vi.mock('../lib/api', () => ({
  suggestRecipients: (...args: unknown[]) => suggestRecipients(...args),
}))

beforeEach(() => {
  suggestRecipients.mockReset()
  suggestRecipients.mockResolvedValue([])
})

const show = (value = '') => {
  const onChange = vi.fn()
  const view = render(
    <RecipientField id="to" accountId={1} value={value} onChange={onChange} />,
  )
  return { ...view, onChange, input: view.getByTestId('to') as HTMLInputElement }
}

describe('what the field shows', () => {
  it('shows finished addresses as pills and keeps the unfinished one in the input', () => {
    const { getByText, input } = show('Alice <alice@example.com>, bob@exa')

    expect(getByText('Alice')).toBeTruthy()
    expect(input.value).toBe('bob@exa')
  })

  it('does not rewrite a line that already has the shape it would produce', async () => {
    const { onChange } = show('Alice <alice@example.com>, bob@example.com')
    await waitFor(() => expect(onChange).not.toHaveBeenCalled())
  })
})

describe('committing an address', () => {
  it('turns a typed comma into a pill', async () => {
    const { getByText, input, onChange } = show()

    fireEvent.change(input, { target: { value: 'alice@example.com,' } })

    expect(getByText('alice@example.com')).toBeTruthy()
    expect(input.value).toBe('')
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith('alice@example.com'))
  })

  it('commits on Enter', async () => {
    const { getByText, input, onChange } = show()

    fireEvent.change(input, { target: { value: 'alice@example.com' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(getByText('alice@example.com')).toBeTruthy()
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith('alice@example.com'))
  })

  // Backspace in an empty input is the quickest way to take the last address
  // back, and it is what every pill field does.
  it('removes the last pill on Backspace when the input is empty', async () => {
    const { queryByText, input, onChange } = show(
      'Alice <alice@example.com>, bob@example.com, ',
    )

    fireEvent.keyDown(input, { key: 'Backspace' })

    expect(queryByText('bob@example.com')).toBeNull()
    expect(queryByText('Alice')).toBeTruthy()
    await waitFor(() =>
      expect(onChange).toHaveBeenLastCalledWith('Alice <alice@example.com>'),
    )
  })

  it('puts a pill back in the input for editing', () => {
    const { getByText, input } = show('Alice <alice@example.com>, bob@example.com, ')

    fireEvent.click(getByText('Alice'))

    expect(input.value).toBe('Alice <alice@example.com>')
    expect(getByText('bob@example.com')).toBeTruthy()
  })

  it('does not add the same address twice', async () => {
    const { getAllByText, input } = show('Alice <alice@example.com>, ')

    fireEvent.change(input, { target: { value: 'Alice <alice@example.com>,' } })

    // The pill that was already there, and no second copy.
    expect(getAllByText('Alice')).toHaveLength(1)
  })
})

describe('completion', () => {
  const alice = { name: 'Alice', addr: 'alice@example.com' }

  it('offers people who wrote to the account once two characters are typed', async () => {
    suggestRecipients.mockResolvedValue([alice])
    const { input, findByRole } = show()

    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'ali' } })

    await waitFor(() => expect(suggestRecipients).toHaveBeenCalledWith(1, 'ali', 8))
    expect(await findByRole('option')).toBeTruthy()
  })

  it('does not ask for a single character', async () => {
    const { input } = show()

    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'a' } })

    await new Promise((resolve) => setTimeout(resolve, 250))
    expect(suggestRecipients).not.toHaveBeenCalled()
  })

  it('puts the chosen person on the line with their name', async () => {
    suggestRecipients.mockResolvedValue([alice])
    const { input, findByText, getByText, onChange } = show()

    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'ali' } })
    fireEvent.click(await findByText('Alice'))

    expect(getByText('Alice')).toBeTruthy()
    expect(input.value).toBe('')
    await waitFor(() =>
      expect(onChange).toHaveBeenLastCalledWith('Alice <alice@example.com>'),
    )
  })

  it('can be driven from the keyboard', async () => {
    suggestRecipients.mockResolvedValue([alice])
    const { input, findByRole } = show()

    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'ali' } })
    await findByRole('option')

    fireEvent.keyDown(input, { key: 'ArrowDown' })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => expect(input.value).toBe(''))
  })

  it('does not offer somebody already on the line', async () => {
    suggestRecipients.mockResolvedValue([alice])
    const { input, queryByRole } = show('Alice <alice@example.com>, ')

    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'ali' } })

    await waitFor(() => expect(suggestRecipients).toHaveBeenCalled())
    expect(queryByRole('option')).toBeNull()
  })

  // Completion is an offer. A failed lookup must not put an error over the
  // message somebody is writing.
  it('stays quiet when the lookup fails', async () => {
    suggestRecipients.mockRejectedValue(new Error('nope'))
    const { input, queryByRole, queryByRole: byRole } = show()

    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'ali' } })

    await waitFor(() => expect(suggestRecipients).toHaveBeenCalled())
    expect(queryByRole('option')).toBeNull()
    expect(byRole('alert')).toBeNull()
  })
})
