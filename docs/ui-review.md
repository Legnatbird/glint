# UI/UX review

## The request

> Listo, lo único es que el texto de la fila donde se hace hover en setting no está centrado,
> pero eso tiene que ver con lo que te voy a pedir ahora, necesito que hagas una revisión
> general de UI/UX de toda la app y la mejores

## How it was reviewed

Every surface was driven and captured instead of read from memory: empty query, typed query,
no results, the synthetic Settings row, a wrapped selection, the settings form, the hotkey
capture state, the last row and an armed destructive row. The two windows have no child
controls, so the review is a review of painting code, hit-testing and key handling.

## Findings

### Defects

| # | What | Evidence | Status |
|---|---|---|---|
| 1 | Settings rows paint their text at a fixed `y+7` while a row is 30px tall, so every label and value sits low in its row; the 44px→50px row change made it worse (the font went 16px→18px). | `state-6-settings`, `state-9-armed` | fixed |
| 2 | Long text is clipped mid-glyph instead of ending in an ellipsis: a manual app's path runs off the settings window, and a long app name runs off the launcher. | `state-7-capture` (`…\GF2Exilium\PCLauncher.exe` cut at the edge) | fixed |
| 3 | With an empty query the caret is painted at the same x as the placeholder, so the block sits on top of the "S" of "Search applications". | `paintQuery` draws both at `l.pad` | fixed |
| 4 | The launcher footer always lists "Enter · Shift+Enter · Ctrl+Enter · Esc", including when there are no results or the selection is the Settings row, where three of the four do nothing. | `state-3-noresults`, `state-4-settings-row` | fixed |
| 5 | "No applications found" says what happened but not what to do, and it reads the same while the first scan is still running. | `state-3-noresults` | fixed |
| 6 | The caret does not restart its blink on a keystroke, so it can be invisible exactly when the user starts typing. | `onChar` never touches `caretOn` | fixed |
| 7 | A second launch of the executable does nothing: it posts `WM_APP+1` and the message loop has no case for it. | measured: hidden before the second launch, hidden after | fixed |
| 8 | A finished scan never reaches the window, so a cold start stays on "Reading the index…" until the next keystroke and a rebuild leaves "Rebuilding index…" in the footer for good. | F5 leaves the status up; `rebuild` had no completion hook | fixed |
| 9 | Progress is painted in the error colour: the footer treated every status as a failure. | "Rebuilding index…" in red | fixed |

### Gaps

| # | What | Why it matters | Status |
|---|---|---|---|
| 10 | The launcher ignores the mouse completely: no hover, no click, no wheel. The settings window has click and wheel, so the two surfaces disagree. | A launcher that only answers the keyboard is a launcher nobody discovers; hovering is the cheapest way to show a row is clickable. | fixed |
| 11 | Settings rows highlight only on keyboard selection; the pointer gives no feedback at all. | Same reason, and it is the "hover" the request names. | fixed |
| 12 | The query was an append/backspace rune list: no caret, no selection, and Escape was the only way to clear it. | Every other text box on the machine answers Ctrl+A, arrows and Home/End; a search box that does not is the one place the user has to relearn typing. | fixed |
| 13 | Value rows (`max_results`, PATH scope) changed only with ←/→, and nothing on screen said so. | An affordance the user cannot see is not an affordance. | fixed |
| 14 | A manual app's name was its file name, with no way to change it; removal and activation shared `Enter`. | The name is what the list shows, so it has to be editable there. | fixed |
| 15 | Copy and paste in the query field. | Reading and writing the Win32 clipboard needs an HGLOBAL-to-Go byte copy; the vet-clean workarounds (self `ReadProcessMemory`/`WriteProcessMemory`) buy a feature with a hack. | deferred, with the reason |

### Checked and left alone

| What | Why |
|---|---|
| Colour contrast | Dim text on the panel is 4.9:1, body text 15:1, accent 7:1 — above AA. Nothing to fix. |
| Destructive rows | Two-press confirmation with an armed colour and a footer hint already works. |
| Scrollbar (settings) | Wheel and keyboard reach everything; a draggable thumb is not worth the code yet. |
| Tray menu | Items match the app's verbs and the autostart item shows a check. |
| Per-monitor DPI | Out of scope by decision (`ae13d33`): single monitor. |

## Rules the fixes follow

- Text is never positioned with a hand-tuned offset: rows centre with the font's own line
  height, so changing a font size cannot move a row's text again.
- Anything that can overflow gets a width and an ellipsis, in both windows.
- The footer describes what the selected row can actually do, and says nothing about keys
  that would do nothing.
- Hover is feedback, not selection: the keyboard selection stays where it is, the pointer
  only shows which row a click would hit.
- Anything that happens off the message thread — a second process, a finished scan —
  reaches the window as a posted message, never by touching UI state directly.
