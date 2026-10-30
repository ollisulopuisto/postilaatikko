<script lang="ts">
  // Full-message templates are stored locally. Selecting one only fills a blank
  // subject and inserts its body at the current cursor, preserving draft fields.
  import Icon from '@iconify/svelte'
  import { onMount } from 'svelte'
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { _ } from '$lib/i18n'
  import { toasts } from '$lib/stores/toast'
  import { dialogGuardOpen, dialogGuardClose } from '$lib/stores/dialogGuard'
  import { ListEmailTemplates, SaveEmailTemplate, DeleteEmailTemplate } from '$wailsjs/go/app/App.js'
  import type { settings } from '$wailsjs/go/models'

  interface Props {
    onInsert: (template: settings.EmailTemplate) => void
    compact?: boolean
  }
  let { onInsert, compact = false }: Props = $props()

  let templates = $state<settings.EmailTemplate[]>([])
  let menuOpen = $state(false)
  let manageOpen = $state(false)
  let editId = $state('')
  let editName = $state('')
  let editSubject = $state('')
  let editBody = $state('')
  let saving = $state(false)

  async function load() {
    try {
      templates = (await ListEmailTemplates()) ?? []
    } catch (err) {
      console.error('Failed to load email templates:', err)
      toasts.error($_('composer.emailTemplatesLoadFailed'))
    }
  }

  onMount(load)

  let guarded = false
  $effect(() => {
    if (manageOpen && !guarded) {
      dialogGuardOpen()
      guarded = true
    }
    if (!manageOpen && guarded) {
      dialogGuardClose()
      guarded = false
    }
  })

  function resetForm() {
    editId = ''
    editName = ''
    editSubject = ''
    editBody = ''
  }

  function edit(template: settings.EmailTemplate) {
    editId = template.id
    editName = template.name
    editSubject = template.subject
    editBody = template.body
  }

  function openManage() {
    resetForm()
    manageOpen = true
  }

  async function save() {
    if (saving) return
    saving = true
    try {
      await SaveEmailTemplate({ id: editId, name: editName, subject: editSubject, body: editBody } as settings.EmailTemplate)
      await load()
      resetForm()
    } catch (err) {
      console.error('Failed to save email template:', err)
      toasts.error($_('composer.emailTemplateSaveFailed'))
    } finally {
      saving = false
    }
  }

  async function remove(id: string) {
    try {
      await DeleteEmailTemplate(id)
      await load()
      if (editId === id) resetForm()
    } catch (err) {
      console.error('Failed to delete email template:', err)
      toasts.error($_('composer.emailTemplateDeleteFailed'))
    }
  }

  const canSave = $derived(editName.trim() !== '' && (editSubject.trim() !== '' || editBody.trim() !== ''))
</script>

<DropdownMenu.Root bind:open={menuOpen} onOpenChange={(open) => { if (open) void load() }}>
  <DropdownMenu.Trigger
    class="{compact ? 'p-1.5 rounded-md' : 'flex items-center gap-1'} hover:text-foreground hover:bg-muted transition-colors"
    title={$_('composer.emailTemplates')}
    aria-label={$_('composer.emailTemplates')}
  >
    <Icon icon="mdi:file-document-edit-outline" class="h-4 w-4" />
    {#if !compact}{$_('composer.emailTemplates')}{/if}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="start" class="max-w-sm">
    {#each templates as template (template.id)}
      <DropdownMenu.Item onSelect={() => onInsert(template)}>
        <span class="min-w-0">
          <span class="block truncate" title={template.name}>{template.name}</span>
          {#if template.subject}<span class="block truncate text-xs text-muted-foreground" title={template.subject}>{template.subject}</span>{/if}
        </span>
      </DropdownMenu.Item>
    {:else}
      <div class="px-2 py-1.5 text-xs text-muted-foreground">{$_('composer.emailTemplatesEmpty')}</div>
    {/each}
    <DropdownMenu.Separator />
    <DropdownMenu.Item onSelect={openManage}>
      <Icon icon="mdi:pencil-outline" class="mr-2 h-4 w-4" />
      {$_('composer.emailTemplatesManage')}
    </DropdownMenu.Item>
  </DropdownMenu.Content>
</DropdownMenu.Root>

<Dialog.Root bind:open={manageOpen}>
  <Dialog.Content class="max-h-[90vh] max-w-xl overflow-y-auto">
    <Dialog.Header>
      <Dialog.Title>{$_('composer.emailTemplatesManage')}</Dialog.Title>
      <Dialog.Description>{$_('composer.emailTemplatesDescription')}</Dialog.Description>
    </Dialog.Header>
    <ul class="space-y-1">
      {#each templates as template (template.id)}
        <li class="flex min-w-0 items-center gap-2 rounded-md px-2 py-1 hover:bg-muted {editId === template.id ? 'bg-muted' : ''}">
          <span class="min-w-0 flex-1">
            <span class="block truncate text-sm" title={template.name}>{template.name}</span>
            {#if template.subject}<span class="block truncate text-xs text-muted-foreground" title={template.subject}>{template.subject}</span>{/if}
          </span>
          <button type="button" class="rounded p-1 hover:bg-background" onclick={() => edit(template)} title={$_('common.edit')} aria-label={$_('common.edit')}>
            <Icon icon="mdi:pencil" class="h-4 w-4" />
          </button>
          <button type="button" class="rounded p-1 text-destructive hover:bg-background" onclick={() => void remove(template.id)} title={$_('common.delete')} aria-label={$_('common.delete')}>
            <Icon icon="mdi:delete-outline" class="h-4 w-4" />
          </button>
        </li>
      {/each}
    </ul>
    <form class="space-y-2" onsubmit={(event) => { event.preventDefault(); void save() }}>
      <Input bind:value={editName} placeholder={$_('composer.emailTemplateName')} aria-label={$_('composer.emailTemplateName')} />
      <Input bind:value={editSubject} placeholder={$_('composer.emailTemplateSubject')} aria-label={$_('composer.emailTemplateSubject')} />
      <textarea
        bind:value={editBody}
        rows="7"
        placeholder={$_('composer.emailTemplateBody')}
        aria-label={$_('composer.emailTemplateBody')}
        class="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      ></textarea>
      <Dialog.Footer>
        {#if editId}<Button type="button" variant="ghost" onclick={resetForm}>{$_('common.cancel')}</Button>{/if}
        <Button type="submit" disabled={!canSave || saving}>{editId ? $_('common.save') : $_('common.add')}</Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
