/**
 * Centralized z-index scale for the application.
 *
 * Usage:
 * - Import where needed: import { zIndex } from './theme/zIndex'
 * - Use as props: zIndex={zIndex.popover}
 *
 * Scale hierarchy (ascending order):
 * - Base layer: editor content
 * - Editor decorations: selections and cursors
 * - UI components: sidebar, footer
 * - Overlays: dropdowns, modals, popovers, toasts
 *
 * ⚠️ Important:
 * - Always use these constants instead of hardcoded numbers
 * - If you need a new layer, add it here and update this comment
 * - Chakra UI modals default to 1400, toasts to 1500
 */
export const zIndex = {
  // Base layer
  base: 0,

  // Editor layers (Monaco editor decorations)
  editorSelection: 1,
  editorCursor: 2,

  // UI components
  sidebar: 10,
  footer: 10,

  // Overlays (in ascending order)
  dropdown: 1000,
  sticky: 1100,
  modal: 1300, // AlertDialog, security modals
  popover: 1400, // User info popover
  toast: 1500, // Chakra toast notifications
  tooltip: 1600,
} as const;

export type ZIndex = typeof zIndex;
