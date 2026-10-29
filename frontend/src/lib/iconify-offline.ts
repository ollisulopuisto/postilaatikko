// Configure Iconify to work offline with the source-generated set of icons
// referenced by the host app and bundled extensions.
import { addCollection } from '@iconify/svelte'
import offlineIcons from './generated/offline-icons.json'

for (const collection of Object.values(offlineIcons)) addCollection(collection)
