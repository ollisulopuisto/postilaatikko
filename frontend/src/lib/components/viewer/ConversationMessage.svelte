<script lang="ts">
  // Body of one message in the conversation timeline (UX-008). It asks the
  // backend to split the stored body into new text, quoted history and a
  // signature, shows the new text, and folds the rest behind buttons. Text the
  // extractor could not classify is never folded, and when the split is
  // unavailable, uncertain or fails, the original message is shown exactly as
  // before. Every piece is still rendered by EmailBody, so link handling,
  // remote-image blocking, inline images and dark mode behave as they always did.
  import Icon from '@iconify/svelte'
  import { GetReplyContent } from '../../../../wailsjs/go/app/App'
  import EmailBody from './EmailBody.svelte'
  import { _ } from '$lib/i18n'
  import { buildBodyPlan, type BodyPlan } from './replyContent'
  import type { ReaderMode } from './readerMode'

  interface Props {
    messageId: string
    accountId: string
    /** What EmailBody would show for this message today (decrypted when it is). */
    bodyHtml?: string
    bodyText?: string
    fromEmail?: string
    /** False for header-only and encrypted messages: they show the original. */
    extractable?: boolean
    /** Traditional shows every message in full and asks for no split. */
    mode?: ReaderMode
    onCompose?: (to: string) => void
    onImagesLoaded?: () => void
    encryptedInlineAttachments?: Record<string, string>
    darken?: boolean
  }

  let {
    messageId,
    accountId,
    bodyHtml = '',
    bodyText = '',
    fromEmail = '',
    extractable = true,
    mode = 'conversation',
    onCompose,
    onImagesLoaded,
    encryptedInlineAttachments,
    darken = false,
  }: Props = $props()

  // null while the split is being prepared.
  let plan = $state<BodyPlan | null>(null)
  let unfolded = $state<Set<string>>(new Set())
  let showFull = $state(false)
  let bodyRefs = $state<Record<string, { getPrintableHtml(): Promise<string> }>>({})

  // A different message starts folded again.
  let lastMessageId = ''
  $effect(() => {
    if (messageId !== lastMessageId) {
      lastMessageId = messageId
      unfolded = new Set()
      showFull = false
    }
  })

  $effect(() => {
    // Re-run when the message or its body changes (for example a late download).
    void bodyHtml
    void bodyText
    const id = messageId
    const account = accountId
    void mode
    if (!extractable || mode === 'traditional') {
      plan = { mode: 'original', reason: 'unavailable' }
      return
    }
    plan = null
    let stale = false
    GetReplyContent(account, id, '', false)
      .then((reply) => {
        if (!stale) plan = buildBodyPlan(reply)
      })
      .catch((err) => {
        // Superseded or failed: show the original rather than nothing.
        console.warn('[ConversationMessage] reply content unavailable:', err)
        if (!stale) plan = { mode: 'original', reason: 'error' }
      })
    return () => {
      stale = true
    }
  })

  function toggle(blockId: string) {
    const next = new Set(unfolded)
    if (!next.delete(blockId)) next.add(blockId)
    unfolded = next
  }

  /** Printable HTML of what is on screen: every shown piece, in order. */
  export async function getPrintableHtml(): Promise<string> {
    const parts: string[] = []
    for (const ref of Object.values(bodyRefs)) {
      try {
        parts.push(await ref.getPrintableHtml())
      } catch {
        // A piece that cannot print is skipped; the others still do.
      }
    }
    return parts.join('')
  }
</script>

{#snippet original()}
  <EmailBody
    bind:this={bodyRefs['original']}
    {messageId}
    {accountId}
    {bodyHtml}
    {bodyText}
    {fromEmail}
    {onCompose}
    {onImagesLoaded}
    {encryptedInlineAttachments}
    {darken}
  />
{/snippet}

{#if plan === null}
  <div class="text-muted-foreground text-sm italic py-4" role="status">
    <Icon icon="mdi:loading" class="w-4 h-4 animate-spin inline-block align-text-bottom" />
    {$_('viewer.preparingMessage')}
  </div>
{:else if plan.mode === 'original' || showFull}
  {#if plan.mode === 'blocks'}
    <div class="mb-1 flex justify-end">
      <button
        type="button"
        class="text-xs text-muted-foreground hover:text-foreground underline underline-offset-2"
        aria-pressed="true"
        data-original-toggle
        onclick={() => (showFull = false)}
        onkeydown={(e) => e.stopPropagation()}
      >
        {$_('viewer.showSplitMessage')}
      </button>
    </div>
  {/if}
  {@render original()}
{:else}
  <div class="mb-1 flex justify-end">
    <button
      type="button"
      class="text-xs text-muted-foreground hover:text-foreground underline underline-offset-2"
      aria-pressed="false"
      data-original-toggle
      onclick={() => (showFull = true)}
      onkeydown={(e) => e.stopPropagation()}
    >
      {$_('viewer.showFullMessage')}
    </button>
  </div>
  <div class="space-y-2" data-reply-content="blocks">
    {#each plan.blocks as block (block.id)}
      {@const open = !block.collapsed || unfolded.has(block.id)}
      {#if block.collapsed}
        <button
          type="button"
          class="flex w-full items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-1.5 text-left text-xs text-muted-foreground hover:bg-muted/60 hover:text-foreground transition-colors"
          aria-expanded={open}
          data-block-kind={block.kind}
          onclick={() => toggle(block.id)}
          onkeydown={(e) => e.stopPropagation()}
        >
          <Icon icon={open ? 'mdi:chevron-up' : 'mdi:dots-horizontal'} class="w-4 h-4 flex-shrink-0" />
          <span class="flex-shrink-0 font-medium">
            {block.kind === 'signature'
              ? (open ? $_('viewer.hideSignature') : $_('viewer.showSignature'))
              : (open ? $_('viewer.hideQuotedText') : $_('viewer.showQuotedText'))}
          </span>
          {#if !open && block.preview}
            <span class="truncate">{block.preview}</span>
          {/if}
        </button>
      {/if}
      {#if open}
        <EmailBody
          bind:this={bodyRefs[block.id]}
          messageId={messageId}
          {accountId}
          bodyHtml={block.format === 'html' ? block.content : ''}
          bodyText={block.format === 'text' ? block.content : ''}
          {fromEmail}
          {onCompose}
          {onImagesLoaded}
          {encryptedInlineAttachments}
          {darken}
        />
      {/if}
    {/each}
  </div>
{/if}
