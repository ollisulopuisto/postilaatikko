<script lang="ts">
 import MessageList from '../../src/lib/components/list/MessageList.svelte'
 import { _ } from '../../src/lib/i18n'
 let mode = $state<'' | 'unread' | 'needsMe' | 'readLater'>('needsMe')
 let folder = $state('inbox-a')
</script>
<div class="flex h-screen flex-col bg-background text-foreground" style="max-width:640px">
 <div class="flex flex-wrap gap-2 border-b border-border p-2">
  <button class="rounded border p-2" aria-pressed={mode===''} onclick={()=>mode=''}>{$_('triage.all')}</button>
  <button class="rounded border p-2" aria-pressed={mode==='unread'} onclick={()=>mode='unread'}>{$_('triage.unread')}</button>
  <button class="rounded border p-2" aria-pressed={mode==='needsMe'} onclick={()=>mode='needsMe'}>{$_('triage.needsMe')}</button>
  <button class="rounded border p-2" aria-pressed={mode==='readLater'} onclick={()=>mode='readLater'}>{$_('triage.readLater')}</button>
 </div>
 <div class="flex flex-wrap gap-2 border-b border-border p-2">
  <button class="rounded border p-2" onclick={()=>folder='inbox-a'}>Inbox fixture</button>
  <button class="rounded border p-2" onclick={()=>folder='projects-a'}>Projects fixture</button>
 </div>
 <div class="min-h-0 flex-1"><MessageList accountId="a" folderId={folder} folderName={folder==='inbox-a'?'Inbox':'Projects'} folderType="folder" triageFilter={mode} showQuickFilters={false}/></div>
</div>
