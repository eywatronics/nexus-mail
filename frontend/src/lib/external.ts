import { Browser } from '@wailsio/runtime'

/**
 * Opens a page in the operating system's browser.
 *
 * Not `window.open`: inside the WebView that either gets blocked or replaces
 * the application's own document, and a mail client that navigates away from
 * itself to show a help page has lost the form the reader was filling in.
 */
export const openExternal = (url: string) => Browser.OpenURL(url)
