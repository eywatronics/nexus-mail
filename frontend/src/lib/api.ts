import { MailService } from '../../bindings/nexusmail/internal/app'

/**
 * One wrapper around the generated bindings.
 *
 * Components never import the bindings directly: this is the single place to
 * fix if the generated path changes, and the single place a test needs to
 * mock.
 */
export interface Account {
  id: number
  email: string
  displayName: string
  provider: string
  authKind: string
}

export interface Folder {
  id: number
  accountId: number
  name: string
  path: string
  totalCount: number
  unreadCount: number
  isInbox: boolean
  /**
   * What the mailbox is for: inbox, sent, drafts, archive, junk, trash, or
   * empty for an ordinary folder. Decided in the backend from the server's
   * special-use attributes, which the window never sees.
   */
  role: string
}

export interface Message {
  id: number
  folderId: number
  uid: number
  threadId: string
  /** How many messages this conversation has here. Zero outside the threaded list. */
  threadCount: number
  subject: string
  fromName: string
  fromAddr: string
  snippet: string
  internalDateUnix: number
  isRead: boolean
  isStarred: boolean
  hasAttachments: boolean
  bodyFetched: boolean
}

/** How much of what the user asked for has not reached the server. */
export interface PendingChanges {
  pending: number
  /** Could not be applied: the server recreated the mailbox they belonged to. */
  dropped: number
  /** Hit something retrying cannot fix. */
  failed: number
}

/** One file carried by a message. */
export interface Attachment {
  id: number
  filename: string
  mimeType: string
  /** What the server reports, which is the encoded size. */
  size: number
  downloaded: boolean
  localPath?: string
}

export interface LogBundle {
  path: string
  bytes: number
  files: number
  preview: string
}

export const listAccounts = () => MailService.ListAccounts() as Promise<Account[]>

export const listFolders = (accountId: number) =>
  MailService.ListFolders(accountId) as Promise<Folder[]>

export const listMessages = (
  folderId: number,
  limit: number,
  offset: number,
  threaded: boolean,
) => MailService.ListMessages(folderId, limit, offset, threaded) as Promise<Message[]>

export const openFolder = (folderId: number, limit: number, threaded: boolean) =>
  MailService.OpenFolder(folderId, limit, threaded) as Promise<Message[]>

export const searchMessages = (accountId: number, query: string, limit: number) =>
  MailService.SearchMessages(accountId, query, limit) as Promise<Message[]>

export const markRead = (messageIds: number[], read: boolean) =>
  MailService.MarkRead(messageIds, read)

export const setStarred = (messageIds: number[], starred: boolean) =>
  MailService.SetStarred(messageIds, starred)

export const deleteMessages = (messageIds: number[]) =>
  MailService.DeleteMessages(messageIds)

export const moveMessages = (messageIds: number[], targetFolderId: number) =>
  MailService.MoveMessages(messageIds, targetFolderId)

export const pendingChanges = (accountId: number) =>
  MailService.PendingChangeCount(accountId) as Promise<PendingChanges>

export const acknowledgeChangeFailures = (accountId: number) =>
  MailService.AcknowledgeChangeFailures(accountId)

export const listAttachments = (messageId: number) =>
  MailService.ListAttachments(messageId) as Promise<Attachment[]>

/** Fetches the bytes if needed and opens the folder they were saved in. */
export const revealAttachment = (attachmentId: number) =>
  MailService.RevealAttachment(attachmentId)

export const syncAccount = (accountId: number) => MailService.SyncAccount(accountId)

export const exportLogs = () => MailService.ExportLogs() as Promise<LogBundle>

export const addPasswordAccount = (
  email: string,
  displayName: string,
  imapHost: string,
  imapPort: number,
  /** "tls" (implicit, 993) or "starttls" (upgrade, 143). Anything else reads as tls. */
  imapSecurity: string,
  smtpHost: string,
  smtpPort: number,
  password: string,
) =>
  MailService.AddPasswordAccount(
    email,
    displayName,
    imapHost,
    imapPort,
    imapSecurity,
    smtpHost,
    smtpPort,
    password,
  ) as Promise<Account>

export const addOAuthAccount = (email: string, displayName: string, provider: string) =>
  MailService.AddOAuthAccount(email, displayName, provider) as Promise<Account>

/**
 * The body is served over HTTP, not the bridge, so this is just a URL.
 *
 * `version` is cache-busting and nothing else — the handler ignores it. The
 * frame is keyed on its URL, so after an encoding repair rewrites the body the
 * frame would otherwise sit on the identical URL and never re-request it.
 */
export const bodyURL = (
  messageId: number,
  allowRemote: boolean,
  version = 0,
  view: 'rich' | 'simple' | 'text' = 'rich',
  /**
   * Which palette the frame should use. The frame is sandboxed and cannot see
   * the class on this page, so the only way it learns the app is in dark mode
   * is by being told here.
   */
  theme?: 'light' | 'dark',
) => {
  const params = new URLSearchParams()
  if (allowRemote) params.set('remote', '1')
  if (version > 0) params.set('v', String(version))
  // The default is omitted rather than spelled out, so an ordinary body URL
  // stays the short one the tests and the logs already show.
  if (view !== 'rich') params.set('view', view)
  if (theme) params.set('theme', theme)

  const query = params.toString()
  return `/mail-body/${messageId}${query ? `?${query}` : ''}`
}

/**
 * The message exactly as it arrived, served as plain text.
 *
 * Also over HTTP rather than the bridge: a message is as large as a message,
 * and the source view is the one place its whole size is on screen at once.
 */
export const sourceURL = (messageId: number) => `/mail-source/${messageId}`

/** Writes the message as an .eml and opens the folder it landed in. */
export const saveMessageAsEML = (messageId: number) =>
  MailService.RevealMessageEML(messageId)

/** One entry in the encoding picker. */
export interface Charset {
  name: string
  /** Names the languages, not the standard: readers know "Turkish", not "Latin-5". */
  label: string
}

export const repairCharsets = () => MailService.RepairCharsets() as Promise<Charset[]>

/** Re-reads a message with a chosen encoding and replaces the stored body. */
export const repairEncoding = (messageId: number, charsetName: string) =>
  MailService.RepairEncoding(messageId, charsetName)

/** What pressing undo would take back, or a zero kind when there is nothing. */
export interface Undoable {
  kind: '' | 'move' | 'delete' | 'trash'
  count: number
  /** When the change goes out and stops being undoable. */
  expiresUnixMs: number
}

export const undoable = () => MailService.Undoable() as Promise<Undoable>

/** Reports whether anything was actually taken back. */
export const undoLastAction = () => MailService.UndoLastAction() as Promise<boolean>

/** Destroys everything in the trash, on the server as well as here. */
export const emptyTrash = (accountId: number) => MailService.EmptyTrash(accountId)

/** Everything config.json holds, as the settings screen sees it. */
export interface AppSettings {
  googleClientId: string
  microsoftClientId: string
  oauthRedirectPort: number
  retentionDays: number
  retentionMaxMessages: number
  notificationPreview: boolean
  undoWindowSeconds: number
}

export const settings = () => MailService.Settings() as Promise<AppSettings>

/** Writes config.json and applies what can be applied without a restart. */
export const updateSettings = (next: AppSettings) => MailService.UpdateSettings(next)
