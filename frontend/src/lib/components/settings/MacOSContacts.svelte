<script lang="ts">
  import { onMount } from 'svelte'
  import { Button } from '$lib/components/ui/button'
  import { _ } from '$lib/i18n'
  import { macOSContactsBackend, type MacOSPerson } from '$lib/macOSContacts'

  let status = $state('unavailable')
  let people = $state<MacOSPerson[]>([])
  let query = $state('')
  let busy = $state(false)
  let browsed = $state(false)
  let truncated = $state(false)
  let error = $state('')
  let requestID = 0
  const authorized = $derived(status === 'authorized' || status === 'limited')

  onMount(() => {
    let active = true
    macOSContactsBackend().GetMacOSContactsStatus().then(value => {
      if (active) status = value
    }).catch(err => {
      if (active) error = String(err)
    })
    return () => { active = false; requestID++ }
  })

  async function browse() {
    const id = ++requestID
    busy = true
    error = ''
    try {
      status = await macOSContactsBackend().GetMacOSContactsStatus()
      if (!authorized) {
        people = []
        browsed = false
        return
      }
      const result = await macOSContactsBackend().SearchMacOSContacts(query, 100)
      if (id !== requestID) return
      people = result.people || []
      truncated = result.truncated
      browsed = true
    } catch (err) {
      if (id === requestID) {
        people = []
        error = String(err)
      }
    } finally {
      if (id === requestID) busy = false
    }
  }

  async function requestAccess() {
    busy = true
    error = ''
    try {
      status = await macOSContactsBackend().RequestMacOSContactsAccess()
      await browse()
    } catch (err) {
      error = String(err)
      status = await macOSContactsBackend().GetMacOSContactsStatus()
    } finally {
      busy = false
    }
  }

  async function openPerson(id: string) {
    try {
      await macOSContactsBackend().OpenMacOSContact(id)
    } catch (err) {
      error = String(err)
    }
  }
</script>

{#if status !== 'unavailable'}
  <section class="space-y-3 rounded-lg border border-border p-3" aria-labelledby="macos-contacts-title">
    <h3 id="macos-contacts-title" class="text-sm font-medium">{$_('macOSContacts.title')}</h3>
    <p class="text-sm text-muted-foreground">{$_('macOSContacts.description')}</p>
    <p class="text-sm" role="status">{$_(`macOSContacts.status.${status}`)}</p>
    {#if status === 'not-determined'}
      <Button size="sm" onclick={requestAccess} disabled={busy}>{$_('macOSContacts.allow')}</Button>
    {/if}
    {#if status === 'denied' || status === 'restricted'}
      <p class="text-sm text-muted-foreground">{$_('macOSContacts.permissionHelp')}</p>
    {/if}
    <form class="flex flex-wrap gap-2" onsubmit={(event) => { event.preventDefault(); browse() }}>
      {#if authorized}
        <input type="search" bind:value={query} aria-label={$_('macOSContacts.search')} placeholder={$_('macOSContacts.search')}
          class="min-w-0 flex-1 rounded-md border border-border bg-background px-2 py-1 text-sm" maxlength="512" />
        <Button size="sm" type="submit" disabled={busy}>{$_('macOSContacts.browse')}</Button>
      {/if}
      {#if !authorized && status !== 'not-determined'}
        <Button size="sm" variant="outline" type="submit" disabled={busy}>{$_('macOSContacts.checkAccess')}</Button>
      {/if}
    </form>
    {#if error}<p class="text-sm text-destructive" role="alert">{error}</p>{/if}
    {#if browsed && authorized}
      {#if people.length === 0}<p class="text-sm text-muted-foreground">{$_('macOSContacts.empty')}</p>{/if}
      <ul class="max-h-72 overflow-y-auto divide-y divide-border">
        {#each people as person (person.id)}
          <li class="flex items-center justify-between gap-3 py-2">
            <div class="min-w-0">
              <p class="truncate text-sm font-medium">{person.name || person.emails[0] || $_('macOSContacts.unnamed')}</p>
              {#each person.emails as email}<p class="truncate text-xs text-muted-foreground">{email}</p>{/each}
            </div>
            <Button size="sm" variant="outline" onclick={() => openPerson(person.id)}>{$_('macOSContacts.open')}</Button>
          </li>
        {/each}
      </ul>
      {#if truncated}<p class="text-xs text-muted-foreground">{$_('macOSContacts.truncated')}</p>{/if}
    {/if}
  </section>
{/if}
