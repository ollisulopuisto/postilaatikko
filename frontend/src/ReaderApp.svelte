<script lang="ts">
  import ConversationViewer from './lib/components/viewer/ConversationViewer.svelte'
  import ToastContainer from './lib/components/ui/toast/ToastContainer.svelte'
  import { OpenComposerWindow } from '../wailsjs/go/app/App'
  import { EventsEmit, WindowSetTitle } from '../wailsjs/runtime/runtime'
  import { onMount } from 'svelte'
  import { toasts } from './lib/stores/toast'
  let { config }: { config: { accountId: string; folderId: string; threadId: string; folderType: string } } = $props()
  async function reply(mode: 'reply' | 'reply-all' | 'forward', messageId: string) {
    try { await OpenComposerWindow(config.accountId, mode, messageId, '', '') }
    catch (error) { toasts.error(String(error)) }
  }
  onMount(() => {
    WindowSetTitle('Postilaatikko')
    const refresh = () => EventsEmit('messages:updated', { accountId: config.accountId, folderId: config.folderId })
    window.addEventListener('focus', refresh)
    return () => window.removeEventListener('focus', refresh)
  })
</script>
<div class="h-screen flex flex-col bg-background text-foreground">
  <ConversationViewer threadId={config.threadId} folderId={config.folderId} accountId={config.accountId} folderType={config.folderType}
    isFocused onReply={reply} onActionComplete={() => EventsEmit('messages:updated', { accountId: config.accountId, folderId: config.folderId })}
    onComposeToAddress={(address) => OpenComposerWindow(config.accountId, 'new', '', '', `mailto:${encodeURIComponent(address)}`)} />
</div>
<ToastContainer />
