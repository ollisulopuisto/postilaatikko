<script lang="ts">
 import { _ } from '$lib/i18n'
 import { Label } from '$lib/components/ui/label'
 let { executionTarget, selectedTarget = $bindable('local') }: {
   executionTarget: {target: string; endpoint: string; ready: boolean; writeAuthorized: boolean} | null
   selectedTarget?: string
 } = $props()
</script>
      <div class="flex flex-col gap-2 rounded-lg border border-border p-3">
        <Label>{$_('filters.executionTarget')}</Label>
        <label class="flex items-center gap-2"><input type="radio" name="filter-target" value="local" bind:group={selectedTarget} disabled={executionTarget?.target !== 'local'} />{$_('filters.targetLocal')}</label>
        <label class="flex items-center gap-2"><input type="radio" name="filter-target" value="helper" bind:group={selectedTarget} disabled={executionTarget?.target !== 'helper'} />{$_('filters.targetHelper')}</label>
        <label class="flex items-center gap-2 text-muted-foreground"><input type="radio" name="filter-target" disabled />Gmail ({$_('filters.targetGmailPending')})</label>
        {#if executionTarget?.endpoint}<p class="text-xs text-muted-foreground break-all">{executionTarget.endpoint}</p>{/if}
        <p class="text-xs text-muted-foreground">{$_('filters.transferUnavailable')}</p>
        {#if executionTarget && (!executionTarget.ready || !executionTarget.writeAuthorized)}
          <p role="alert" class="filter-target-warning text-xs">{$_('filters.targetUnavailable')}</p>
        {:else if executionTarget}
          <p class="text-xs text-muted-foreground">{$_('filters.targetReady')}</p>
        {/if}
      </div>

<style>
  .filter-target-warning { color: #92400e; }
  :global([data-theme="dark"]) .filter-target-warning { color: #fde68a; }
</style>
