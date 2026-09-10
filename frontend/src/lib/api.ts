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
