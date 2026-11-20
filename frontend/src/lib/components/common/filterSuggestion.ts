export interface FilterSuggestion {
  key: string
  accountId: string
  sourceFolderId: string
  destFolderId: string
  destAccountId: string
  fromContains: string[]
  toContains: string[]
  subjectContains: string[]
}
