<script lang="ts">
 import SelectionObject from '../../src/lib/components/list/SelectionObject.svelte'
 import ActionBar from '../../src/lib/components/kit/ActionBar.svelte'
 let container: HTMLDivElement | null = $state(null)
 import Row from '../../src/lib/components/list/ConversationRow.svelte'
 let checked = $state(new Set<string>())
 const rows = ['First selected target', 'Unchecked gap', 'Second selected target', 'Outside target']
</script>
<div class="bg-background text-foreground p-4" style="width:100%;max-width:600px">
<button onclick={()=>checked = new Set()}>Clear fixture selection</button>
<div class="relative" style="height:500px">
{#if checked.size}<SelectionObject {container} count={checked.size}><ActionBar attached count={checked.size} actions={[{id:'archive',label:'Archive',icon:'mdi:archive'}]} onAction={()=>{(window as any).fixtureAction=checked.size}} onMove={()=>{}} onClear={()=>checked=new Set()}/></SelectionObject>{/if}
<div bind:this={container} style="height:500px;overflow:auto">
{#each rows as subject,index (subject)}
 <Row conversation={{threadId:String(index),subject,snippet:'Synthetic message, no real mail',participants:[{name:'Example sender',email:'sender@example.test'}],latestDate:'2026-11-24T00:00:00Z',messageCount:1,unreadCount:1,messageIds:['m'+index]}} selected={false} checked={checked.has(String(index))} selectionActive={checked.size > 0} selectionFooter={String(index) === [...checked].at(-1)} accountId="a" folderId="inbox" folderType="inbox" selectedMessageIds={[]} selectedIsStarred={false} selectedIsRead={false} onSelect={()=>{}} onCheck={(value)=>{const next=new Set(checked); if(value)next.add(String(index));else next.delete(String(index));checked=next}} onClearSelection={()=>checked = new Set()}/>
{/each}
</div></div></div>
