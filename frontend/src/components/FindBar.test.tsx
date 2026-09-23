import { fireEvent, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FindBar } from './FindBar'

function setup(overrides: Partial<Parameters<typeof FindBar>[0]> = {}) {
  const props = {
    query: '',
    onQueryChange: vi.fn(),
    count: null as number | null,
    index: 0,
    onStep: vi.fn(),
    onClose: vi.fn(),
    ...overrides,
  }
  return { ...render(<FindBar {...props} />), props }
}

describe('opening', () => {
  // A find bar that has to be clicked before it can be typed into is one the
  // keyboard shortcut only half opened.
  it('takes the cursor when it appears', () => {
    const { getByLabelText } = setup()
    expect(document.activeElement).toBe(getByLabelText('Find in message'))
  })

  // Reopening on an existing query should let the next thing typed replace it,
  // which is what selecting it does.
  it('selects an existing query so typing replaces it', () => {
    const { getByLabelText } = setup({ query: 'fatura' })
    const input = getByLabelText('Find in message') as HTMLInputElement

    expect(input.selectionStart).toBe(0)
    expect(input.selectionEnd).toBe('fatura'.length)
  })
})

describe('what it reports', () => {
  it('says nothing until there is a query', () => {
    const { getByTestId } = setup()
    expect(getByTestId('find-count').textContent).toBe('')
  })

  // Null means the document has not answered yet. Showing "no matches" then
  // would be claiming a result nobody has produced.
  it('shows that it is still working rather than claiming a miss', () => {
    const { getByTestId } = setup({ query: 'fatura', count: null })
    expect(getByTestId('find-count').textContent).not.toContain('No matches')
  })

  it('counts from one, because people do', () => {
    const { getByTestId } = setup({ query: 'fatura', count: 12, index: 0 })
    expect(getByTestId('find-count').textContent).toBe('1/12')
  })

  it('names a miss plainly', () => {
    const { getByTestId } = setup({ query: 'fatura', count: 0 })
    expect(getByTestId('find-count').textContent).toBe('No matches')
  })

  // The count is announced as well as drawn, so a reader who cannot see it is
  // told the same thing.
  it('announces the result to a screen reader', () => {
    const { getByTestId } = setup({ query: 'fatura', count: 0 })
    expect(getByTestId('find-count').getAttribute('aria-live')).toBe('polite')
  })
})

describe('moving between matches', () => {
  it('steps forward on Enter and back on Shift+Enter', () => {
    const { getByLabelText, props } = setup({ query: 'fatura', count: 3 })
    const input = getByLabelText('Find in message')

    fireEvent.keyDown(input, { key: 'Enter' })
    expect(props.onStep).toHaveBeenCalledWith(1)

    fireEvent.keyDown(input, { key: 'Enter', shiftKey: true })
    expect(props.onStep).toHaveBeenCalledWith(-1)
  })

  it('steps from the arrows too', () => {
    const { getByLabelText, props } = setup({ query: 'fatura', count: 3 })

    fireEvent.click(getByLabelText('Next match'))
    fireEvent.click(getByLabelText('Previous match'))

    expect(props.onStep).toHaveBeenNthCalledWith(1, 1)
    expect(props.onStep).toHaveBeenNthCalledWith(2, -1)
  })

  // Buttons that would do nothing are disabled rather than left live, so
  // pressing one is never a click that silently fails.
  it('disables the arrows with nothing to step through', () => {
    const { getByLabelText } = setup({ query: 'fatura', count: 0 })

    expect((getByLabelText('Next match') as HTMLButtonElement).disabled).toBe(true)
    expect((getByLabelText('Previous match') as HTMLButtonElement).disabled).toBe(true)
  })
})

describe('closing', () => {
  it('closes on Escape from inside the box', () => {
    const { getByLabelText, props } = setup({ query: 'fatura' })

    fireEvent.keyDown(getByLabelText('Find in message'), { key: 'Escape' })
    expect(props.onClose).toHaveBeenCalled()
  })

  it('closes from the button', () => {
    const { getByLabelText, props } = setup()

    fireEvent.click(getByLabelText('Close find bar'))
    expect(props.onClose).toHaveBeenCalled()
  })
})
