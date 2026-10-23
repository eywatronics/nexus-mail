import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RichTextEditor } from './RichTextEditor'

const execCommand = vi.fn()

beforeEach(() => {
  execCommand.mockReset()
  execCommand.mockReturnValue(true)
  Object.defineProperty(document, 'execCommand', {
    configurable: true,
    value: execCommand,
  })
})

const show = (initialHTML = '', onChange = vi.fn(), onPickImage?: () => Promise<any>) => {
  const view = render(
    <RichTextEditor
      initialHTML={initialHTML}
      onChange={onChange}
      onPickImage={onPickImage}
    />,
  )
  return { ...view, onChange, editor: view.getByTestId('rich-editor') }
}

describe('the editor', () => {
  it('starts with sanitised HTML', () => {
    const { editor } = show('<p onclick="x()">merhaba</p><script>bad()</script>')
    expect(editor.innerHTML).toBe('<p>merhaba</p>')
  })

  it('reports both the HTML and the plain text as it is typed', () => {
    const { editor, onChange } = show('<p>eski</p>')
    editor.innerHTML = '<p>yeni <b>metin</b></p>'
    fireEvent.input(editor)

    expect(onChange).toHaveBeenLastCalledWith(
      '<p>yeni <b>metin</b></p>',
      'yeni metin',
    )
  })

  it('shows a placeholder only while it is empty', () => {
    const { editor, getByText, queryByText, onChange } = show()
    expect(getByText('Write a message')).toBeTruthy()

    editor.innerHTML = '<p>metin</p>'
    fireEvent.input(editor)
    expect(queryByText('Write a message')).toBeNull()
    expect(onChange).toHaveBeenCalled()
  })
})

describe('the toolbar', () => {
  it('runs the command the button names', () => {
    const { getByTestId } = show('<p>metin</p>')

    fireEvent.click(getByTestId('rich-bold'))
    expect(execCommand).toHaveBeenCalledWith('bold', false, undefined)

    fireEvent.click(getByTestId('rich-bullets'))
    expect(execCommand).toHaveBeenCalledWith('insertUnorderedList', false, undefined)
  })

  it('adds a link from the address typed into the panel', () => {
    const { getByTestId } = show('<p>metin</p>')

    fireEvent.click(getByTestId('rich-link'))
    fireEvent.change(getByTestId('rich-link-url'), {
      target: { value: 'example.com' },
    })
    fireEvent.click(getByTestId('rich-link-apply'))

    expect(execCommand).toHaveBeenCalledWith(
      'insertHTML',
      false,
      '<a href="https://example.com">https://example.com</a>',
    )
  })

  it('refuses a link it would not send', () => {
    const { getByTestId, getByText } = show('<p>metin</p>')

    fireEvent.click(getByTestId('rich-link'))
    fireEvent.change(getByTestId('rich-link-url'), {
      target: { value: 'javascript:alert(1)' },
    })
    fireEvent.click(getByTestId('rich-link-apply'))

    expect(getByText(/Use an http/)).toBeTruthy()
    expect(execCommand).not.toHaveBeenCalledWith('insertHTML', false, expect.anything())
  })
})

describe('images', () => {
  const image = {
    cid: 'nx-1@nexus-mail',
    name: 'grafik.png',
    url: '/compose-image/nx-1@nexus-mail',
  }

  it('inserts the image the picker returned', async () => {
    const onPickImage = vi.fn().mockResolvedValue(image)
    const { getByTestId } = show('', vi.fn(), onPickImage)

    fireEvent.click(getByTestId('rich-image'))

    await waitFor(() =>
      expect(execCommand).toHaveBeenCalledWith(
        'insertHTML',
        false,
        `<img src="${image.url}" alt="${image.name}" data-cid="${image.cid}">`,
      ),
    )
  })

  it('inserts nothing when the dialog is cancelled', async () => {
    const onPickImage = vi.fn().mockResolvedValue(null)
    const { getByTestId } = show('', vi.fn(), onPickImage)

    fireEvent.click(getByTestId('rich-image'))
    await waitFor(() => expect(onPickImage).toHaveBeenCalled())

    expect(execCommand).not.toHaveBeenCalledWith(
      'insertHTML',
      false,
      expect.stringContaining('<img'),
    )
  })
})

describe('pasting', () => {  it('sanitises pasted HTML before inserting it', () => {
    const { editor } = show()

    fireEvent.paste(editor, {
      clipboardData: {
        getData: (type: string) =>
          type === 'text/html' ? '<b onclick="bad()">merhaba</b>' : 'merhaba',
      },
    })

    expect(execCommand).toHaveBeenCalledWith('insertHTML', false, '<b>merhaba</b>')
  })

  it('turns pasted plain text into paragraphs', () => {
    const { editor } = show()

    fireEvent.paste(editor, {
      clipboardData: {
        getData: (type: string) => (type === 'text/plain' ? 'bir\n\niki' : ''),
      },
    })

    expect(execCommand).toHaveBeenCalledWith('insertHTML', false, '<p>bir</p><p>iki</p>')
  })
})
