// Which conversations the shared Snooze dialog is open for. The dialog itself
// (SnoozeDialog.svelte) performs the snooze; callers only supply targets.

export interface SnoozeTarget {
  accountId: string
  conversationId: string
}

function createSnoozeDialog() {
  let targets = $state<SnoozeTarget[]>([])
  let open = $state(false)
  return {
    get targets() { return targets },
    get open() { return open },
    set open(value: boolean) { open = value },
    /** Open the dialog for real, account-qualified conversations; unified/virtual placeholders are dropped. */
    show(next: SnoozeTarget[]) {
      const usable = next.filter(t => t.accountId && t.accountId !== 'unified' && t.accountId !== 'virtual' && t.conversationId)
      if (usable.length === 0) return false
      targets = usable
      open = true
      return true
    },
  }
}

export const snoozeDialog = createSnoozeDialog()
