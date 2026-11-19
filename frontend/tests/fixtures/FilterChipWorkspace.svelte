<script lang="ts">
 import MessageList from '../../src/lib/components/list/MessageList.svelte'
 import { accountStore } from '../../src/lib/stores/accounts.svelte'
 let list: MessageList
 let selected = $state('')
 $effect(() => {
   void accountStore
 })
</script>
<div class="flex h-screen bg-background text-foreground">
 <div style="width:460px;min-width:0" class="shrink-0 border-r border-border"><MessageList bind:this={list} accountId="a" folderId="inbox-a" folderName="Inbox" folderType="inbox" onConversationSelect={(thread, folder, account) => selected = `${account}:${folder}:${thread}`} /></div>
 <div class="flex-1 p-8">
   <button class="sr-only" onclick={() => list.setQuickFilter('unread')}>Triage unread</button>
   <button class="sr-only" onclick={() => list.setQuickFilter('readLater')}>Triage later</button>
   <button class="sr-only" onclick={() => list.applyQuickFilter('unread')}>Keyboard unread</button>
   <button class="sr-only" onclick={() => list.setQuickFilter('')}>Clear fixture</button>
   <p>{selected || 'No selection'}</p>
 </div>
</div>
