<script lang="ts">
 import {onMount} from 'svelte'
 import type {Snippet} from 'svelte'
 import {selectionRuns, type SelectionRun} from './selectionGeometry'
 let {container, count, children}: {container: HTMLDivElement | null; count: number; children: Snippet} = $props()
 let runs = $state<SelectionRun[]>([])
 let barTop = $state(0)
 let root: HTMLDivElement | undefined = $state()
 let surface: HTMLDivElement | undefined = $state()
 let frame = 0
 function measure() {
   if (!container || !surface) return
   const bounds = container.getBoundingClientRect()
   const origin = root?.getBoundingClientRect().top ?? bounds.top
   runs = selectionRuns([...container.querySelectorAll<HTMLElement>('[data-conversation-row]')].map(row=>{
     const rect=row.getBoundingClientRect()
     return {top:rect.top-origin,bottom:rect.bottom-origin,checked:row.dataset.bulkChecked==='true'}
   }))
   const barHeight=surface.offsetHeight || 44
   const end=runs.at(-1)?.bottom ?? bounds.height
   // Pin to the visible edge when the selected footer is outside the viewport.
   barTop=Math.max(bounds.top-origin,Math.min(end-barHeight,bounds.bottom-origin-barHeight))
 }
 function schedule() { cancelAnimationFrame(frame); frame=requestAnimationFrame(measure) }
 $effect(()=>{void count;void container;schedule()})
 onMount(()=>{
   const observer=new ResizeObserver(schedule)
   const mutations=new MutationObserver(schedule)
   if(container){observer.observe(container);mutations.observe(container,{subtree:true,childList:true,attributes:true,attributeFilter:['data-bulk-checked']});container.addEventListener('scroll',schedule)}
   if(surface)observer.observe(surface)
   window.addEventListener('resize',schedule);schedule()
   return()=>{observer.disconnect();mutations.disconnect();container?.removeEventListener('scroll',schedule);window.removeEventListener('resize',schedule);cancelAnimationFrame(frame)}
 })
</script>
<div bind:this={root} class="selection-object pointer-events-none absolute inset-0 z-30 overflow-hidden" data-selection-object>
 {#each runs as run, index (index)}
  <div class="selection-run absolute left-1 right-1 rounded-xl border border-primary bg-primary/5" style:top="{run.top}px" style:height="{Math.max(0,run.bottom-run.top)}px"></div>
 {/each}
 <div bind:this={surface} class="selection-object-bar pointer-events-auto absolute left-1 right-1" style:top="{barTop}px">
  {@render children()}
 </div>
</div>
