# Link context menu window bounds

Main37b657cc and open/recent PRs checked before starting, no overlap. EmailBody
used raw click coordinates for fixed menu position without measuring or flipping.
URL hover box did the same and remained visible during context menu. Both
iframe-translated coordinates and plain-text direct events share this problem.

New viewportPopup Svelte action measures after mount, flips up/left on overflow
and clamps to8px inside visual viewport (window fallback). max-content sizing
avoids position-dependent shrink-to-fit feedback; width/height cap keeps oversized
menus scrollable. ResizeObserver handles translated label/content growth; window
and visual viewport resize/scroll re-position, with cleanup on destroy. URL hover
uses same placement and hides while menu is open. Menu actions, privacy URLs,
clipboard/open behavior and iframe coordinate mapping unchanged.

Failing-first real Chrome test failed on tooltip bounds before implementation.
Real Svelte EmailBody browser fixture covers Finnish all-action iframe menu at
bottom/right, suppressed hover box, small-window resize and scroll to last action,
and plain-text link path. Unit cases cover flip/clamp/outside anchors/visual offsets.
1139 frontend tests pass,34skip (browser tests opt-in); new browser test passes
explicitly. check passes0errors with1existing warning. Production Vite build passes
with existing chunk-size advisory. Actual dark Finnish menu pixels inspected at
940x700 and360x240 plus plain-text path. Native macOS WebKit not exercised here.
Own downstream review traced all menu open/close paths, tooltip event race,
reactive anchor updates, ResizeObserver sizing stability, small-window scrolling,
language variation, action cleanup and unchanged link privacy consumers.
No backend changes, app deployment or merge.
