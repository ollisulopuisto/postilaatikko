<script lang="ts">
 import MessageList from '../../src/lib/components/list/MessageList.svelte'
</script>
<div class="flex h-screen flex-col bg-background text-foreground" style="max-width:420px">
 <div class="min-h-0 flex-1"><MessageList accountId="demo" folderId="inbox-a" folderName="Inbox" folderType="inbox" showQuickFilters={false}/></div>
</div>
