# Palette's Journal

Critical UX and Accessibility learnings only.

## 2026-11-08 - Focus-Within Disclosure and Radio Semantics on Account Identity Lists
**Learning:** In account settings identity lists (`AccountIdentityTab.svelte`), row action button wrappers styled with `opacity-0 group-hover:opacity-100` hid Edit and Delete buttons entirely during keyboard tabbing because the parent row had no focus-within disclosure (`group-focus-within:opacity-100 focus-within:opacity-100`). Furthermore, custom radio buttons selecting the default identity lacked `role="radio"`, `aria-checked`, dynamic accessible labels, and keyboard focus rings.
**Action:** Pair hover-revealed action containers with `group-focus-within:opacity-100 focus-within:opacity-100`, supply `aria-label` to all icon buttons, and ensure custom radio toggles include `role="radio"`, `aria-checked`, dynamic labels, and `focus-visible:ring-2` styling.

## 2026-11-07 - Menu Disclosures and Focus Rings on Hover-Revealed Sidebar Actions
**Learning:** Icon-only menu triggers concealed by default via `opacity-0 group-hover:opacity-100` (such as account context menu buttons in `AccountSection.svelte`) previously relied on legacy `focus:opacity-100` without `focus-visible` ring styles, accessible name labels, or popup semantics (`aria-haspopup="menu"`, `aria-expanded`). This left keyboard navigation without an outline and screen readers without announcement of the action or its expanded state.
**Action:** Always provide `aria-haspopup="menu"` and `aria-expanded` on menu disclosure triggers, pair hover disclosure classes with `focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring`, and supply localized `aria-label` and `title` attributes.

## 2026-11-06 - Focus-Visible States on Hover-Revealed Attachment Action Controls
**Learning:** Secondary actions hidden by default with `opacity-0 group-hover:opacity-100` (such as attachment Open, Download, and Remove buttons) become invisible obstacles for keyboard navigation unless `focus-visible:opacity-100` is explicitly set along with focus ring classes. Additionally, icon-only buttons with `title` attributes must also provide explicit `aria-label` properties to guarantee consistent screen reader announcements.
**Action:** Whenever using `opacity-0 group-hover:opacity-100` on interactive controls, always pair it with `focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring` and explicit `aria-label` attributes.

## 2026-11-01 - Focus-Visible Opacity and Outline on Sidebar Card Action Buttons
**Learning:** In sidebar card rows (`NavigationCards.svelte`), action buttons rendered conditionally (such as edit and delete controls) must consistently specify `focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring` and meaningful `title` attributes. Without `focus-visible:opacity-100`, hover-revealed or subtle action controls can fail to render a visible focus state during keyboard tab navigation.
**Action:** When adding or editing secondary row action buttons in list and card components, always ensure `focus-visible:opacity-100` and the standard focus ring classes are included along with matching `aria-label` and `title` attributes.

## 2026-11-08 - Immediate Visual Feedback and Operation-Specific Exit Animations
**Learning:** Abrupt DOM element removal after async backend mutations leaves the user uncertain whether an action registered before messages vanish suddenly. Immediate reactive state changes (dimming, directional translations, and height squishing) paired with an exit-grace delay give tactile feedback.
**Action:** When performing item removal operations (archive, trash, move, spam), track in-flight IDs in a reactive store, immediately apply operation-specific keyframe animations with height collapse, and delay list DOM reload until the animation duration completes. Always include `@media (prefers-reduced-motion: reduce)` fallbacks.
