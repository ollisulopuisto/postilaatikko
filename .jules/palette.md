# Palette's Journal

Critical UX and Accessibility learnings only.

## 2026-11-11 - Focus Rings and Keyboard Glyph Disclosure on Custom Window Controls
**Learning:** In custom window decorations (`WindowControls.svelte`), traffic light controls without explicit focus rings rely on browser defaults that clip or disappear against colored circles, while revealing glyph symbols (`×`, `−`, `+`) strictly on mouse hover leaves keyboard users navigating blind dots without visual indication of which button performs which action. Additionally, omitting `role="group"` and `aria-label` prevents assistive tech from identifying the controls as a cohesive window management cluster.
**Action:** Pair custom window traffic light buttons with `focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1`, enclose them in a container with `role="group"` and `aria-label`, and disclose action glyphs on focus (`isHovering || isFocused`) as well as mouse hover.

## 2026-11-10 - Checkbox Semantics and Focus-Within Disclosure on Conversation Rows
**Learning:** In conversation list items (`ConversationRow.svelte`), custom selection buttons without explicit `role="checkbox"` and `aria-checked` fail to announce their selection state to screen reader users, while relying on parent hover (`group-hover:opacity-100 group-hover:pointer-events-auto`) on quick-action buttons traps keyboard navigation behind invisible or unresponsive elements.
**Action:** Always provide `role="checkbox"`, `aria-checked`, and contextual `aria-label` on custom row selection toggles, and pair hover-revealed action containers with `group-focus-within:opacity-100 focus-within:opacity-100 group-focus-within:pointer-events-auto focus-within:pointer-events-auto` along with standard `focus-visible:ring-2` focus rings on each button.

## 2026-11-09 - Focus Rings on Hover-Revealed Favorite and Row Action Buttons
**Learning:** While `focus-visible:opacity-100` makes hover-revealed or dimmed buttons (`opacity-0 group-hover:opacity-100`) visible when keyboard focus reaches them, omitting explicit focus ring tokens (`focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring`) leaves them with either low-contrast default browser outlines or subtle bounds that blend into dark card backgrounds.
**Action:** Pair every `focus-visible:opacity-100` reveal on icon buttons with `focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring` to provide an unambiguous, high-contrast focus indicator in both light and dark themes.

## 2026-11-08 - Conversation Bubble Avatar Alignment and Tail Proximity
**Learning:** In conversation view modes, speech bubbles have directional tails pointing toward the speaker. Rendering sender avatars inside an expandable message header rather than adjacent to the speech bubble creates visual disconnect, as users expect avatar photos to ground the bubble directly beside its speech tail.
**Action:** In conversation bubble layouts, align sender avatars beside the speech bubble using a bottom-aligned flex row container (`items-end`), placing the avatar on the left for incoming messages and on the right for outgoing messages to align directly with the bubble's tail.

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
