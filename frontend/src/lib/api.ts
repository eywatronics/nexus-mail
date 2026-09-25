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
/**
 * What the find bar is looking for, and which match it is on.
 *
 * The search travels in the URL because the frame has no scripts to run one.
 * The backend wraps the matches before serving the document and puts an id on
 * the current one, which the fragment then scrolls to — the only way to move a
 * scriptless document.
 */
export interface FindRequest {
  query: string
  index: number
}

/** The id the backend puts on the current match. Mirrors internal/mailhtml. */
const CURRENT_MATCH_ID = 'nx-find-current'

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
  find?: FindRequest,
) => {
  const params = new URLSearchParams()
  if (allowRemote) params.set('remote', '1')
  if (version > 0) params.set('v', String(version))
  // The default is omitted rather than spelled out, so an ordinary body URL
  // stays the short one the tests and the logs already show.
  if (view !== 'rich') params.set('view', view)
  if (theme) params.set('theme', theme)

  const searching = find !== undefined && find.query.trim() !== ''
  if (searching) {
    params.set('find', find.query)
    params.set('findIndex', String(find.index))
  }

  const query = params.toString()
  // The fragment is only added while searching. On a document with no matches
  // it would resolve to nothing, which is harmless — but it would also be one
  // more thing in the URL of every message nobody is searching.
  const fragment = searching ? `#${CURRENT_MATCH_ID}` : ''
  return `/mail-body/${messageId}${query ? `?${query}` : ''}${fragment}`
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

/**
 * What redo would do again, in the same shape and on the same clock.
 *
 * Redo has no deadline of its own — undo's is the moment the queue takes the
 * change — but it expires on the same window anyway: both are a moment of
 * hesitation, and a redo still live much later would be a keystroke that
 * silently deletes mail the reader had decided to keep.
 */
export const redoable = () => MailService.Redoable() as Promise<Undoable>

export const redoLastAction = () => MailService.RedoLastAction() as Promise<boolean>

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
  /**
   * Whether the app starts with the machine.
   *
   * Not in config.json like the rest: it lives in the operating system, and
   * the operating system is its source of truth. Somebody can turn it off in
   * Task Manager or System Settings and the app has to agree with them.
   */
  startAtLogin: boolean
  /** False when the setting could not be read, which is not the same as off. */
  startAtLoginAvailable: boolean
}

export const settings = () => MailService.Settings() as Promise<AppSettings>

/** Writes config.json and applies what can be applied without a restart. */
export const updateSettings = (next: AppSettings) => MailService.UpdateSettings(next)

/** One address an account can send as. */
export interface Identity {
  id: number
  accountId: number
  email: string
  displayName: string
  /** The two together, as the recipient will see them in the From header. */
  from: string
  isDefault: boolean
}

export const identities = (accountId: number) =>
  MailService.Identities(accountId) as Promise<Identity[]>

/** A message as the composer hands it over. Addresses are text, as typed. */
export interface Draft {
  identityId: number
  accountId: number
  to: string
  cc: string
  bcc: string
  subject: string
  text: string
  html: string
  inReplyTo: string
  references: string[]
}

/** What the window is told about a message on its way. */
export interface Queued {
  operationId: number
  /** How many addresses it will be delivered to, blind copies included. */
  recipients: number
}

/**
 * Queues a message. It is not sent by the time this resolves.
 *
 * The backend writes it to disk and returns; it goes out when the connection
 * allows. Waiting for the server here would mean a composer that hangs for the
 * length of a handshake and an upload, and a message lost if it were closed.
 */
export const sendMessage = (draft: Draft) => MailService.SendMessage(draft) as Promise<Queued>

/** A composer opened on an existing message. */
export interface ReplyDraft {
  accountId: number
  to: string
  cc: string
  subject: string
  inReplyTo: string
  references: string[]
  /** The original, marked up and ready to sit under the reply. */
  quoted: string
}

/**
 * Builds the draft for answering a message.
 *
 * Assembled in the backend because every part needs something the window does
 * not have: the Message-ID and References for threading, the full address
 * lists, and the body — which the window holds only as sanitised HTML inside a
 * sandboxed frame it cannot read back.
 */
export const replyDraft = (messageId: number, all: boolean) =>
  MailService.ReplyDraft(messageId, all) as Promise<ReplyDraft>

export const forwardDraft = (messageId: number) =>
  MailService.ForwardDraft(messageId) as Promise<ReplyDraft>
