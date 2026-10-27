import { getUIState, saveUIState } from './uiState.svelte'
import { normalizeCards, moveEntry, moveCard, quickEntries, type NavigationEntry, type NavigationCards } from '../sidebar/cards'
let entries = $state<NavigationEntry[]>([])
let state = $state<NavigationCards>(normalizeCards('', []))
function commit(next: NavigationCards) { state = next; saveUIState({ sidebarCards: JSON.stringify(next) }) }
export const navigationCards = {
  get state() { return state },
  get entries() { return entries },
  get quick() { return quickEntries(state, entries) },
  sync(next: NavigationEntry[]) { entries = next; state = normalizeCards(getUIState().sidebarCards, next) },
  update: commit,
  moveEntry(id: string, card: string, index: number) { commit(moveEntry(state, id, card, index)) },
  moveCard(id: string, delta: number) { commit(moveCard(state, id, delta)) },
  toggleQuick(id: string) { commit({ ...state, quick: state.quick.includes(id) ? state.quick.filter(e => e !== id) : [...state.quick, id].slice(0, 10) }) },
  patchCard(id: string, patch: Partial<NavigationCards['cards'][number]>) { commit({ ...state, cards: state.cards.map(c => c.id === id ? { ...c, ...patch } : c) }) },
  navigate(entry: NavigationEntry) { window.dispatchEvent(new CustomEvent('postilaatikko:navigate', { detail: entry })) },
}
