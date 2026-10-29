import './app.css'
import './lib/iconify-offline'
import { mount } from 'svelte'
import { initI18n } from './lib/i18n'
import MailWindowApp from './MailWindowApp.svelte'
import { loadSettings } from './lib/stores/settings.svelte'
import { loadUIState } from './lib/stores/uiState.svelte'
import { initTheme } from './lib/stores/theme.svelte'
import { loadImageAllowlist } from './lib/stores/imageAllowlist.svelte'
import { createMailWindowBackend, type MailScope } from './lib/windows/mailWindow'
import { GetSystemTheme } from '../wailsjs/go/app/App'

interface MailBackend {
  GetConfig(): Promise<MailScope>
  Call(operation: string, args: unknown[]): Promise<unknown>
}
async function bootstrap() {
  const host = window as unknown as { go: { app: { ReaderApp: MailBackend; App: unknown } } }
  const deadline = Date.now() + 10000
  while (!host.go?.app?.ReaderApp) {
    if (Date.now() > deadline) throw new Error('Mail window runtime unavailable')
    await new Promise(resolve => setTimeout(resolve, 25))
  }
  const backend = host.go.app.ReaderApp
  const config = await backend.GetConfig()
  const initialState = await backend.Call('GetUIState', [])
  host.go.app.App = createMailWindowBackend(backend, initialState)
  await initI18n()
  const theme = await loadSettings()
  await Promise.all([loadUIState(), loadImageAllowlist()])
  await initTheme(theme, GetSystemTheme)
  mount(MailWindowApp, { target: document.getElementById('app')!, props: { config } })
  document.getElementById('boot-status')?.remove()
}
bootstrap().catch(error => {
  console.error('[mail-window]', error)
  const status = document.getElementById('boot-status') ?? document.getElementById('app')!
  status.textContent = `Postilaatikko: ${error instanceof Error ? error.message : String(error)}`
})
