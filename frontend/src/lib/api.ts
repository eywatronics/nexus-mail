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
}

export interface Message {
  id: number
  folderId: number
  uid: number
  threadId: string
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

export const listMessages = (folderId: number, limit: number, offset: number) =>
  MailService.ListMessages(folderId, limit, offset) as Promise<Message[]>

export const openFolder = (folderId: number, limit: number) =>
  MailService.OpenFolder(folderId, limit) as Promise<Message[]>

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
  smtpHost: string,
  smtpPort: number,
  password: string,
) =>
  MailService.AddPasswordAccount(
    email,
    displayName,
    imapHost,
    imapPort,
    smtpHost,
    smtpPort,
    password,
  ) as Promise<Account>

export const addOAuthAccount = (email: string, displayName: string, provider: string) =>
  MailService.AddOAuthAccount(email, displayName, provider) as Promise<Account>

/** The body is served over HTTP, not the bridge, so this is just a URL. */
export const bodyURL = (messageId: number, allowRemote: boolean) =>
  `/mail-body/${messageId}${allowRemote ? '?remote=1' : ''}`

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
