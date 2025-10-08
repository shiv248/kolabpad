/**
 * Centralized layout dimensions and spacing constants.
 *
 * Usage:
 * - Import: import { layout } from './theme/layout'
 * - Use as props: h={layout.footer.height}
 *
 * Benefits:
 * - Consistent spacing across components
 * - Easy to adjust layout globally
 * - Self-documenting code
 *
 * ⚠️ Reserved space for future security features:
 * - lockIndicator: Document lock/password indicator (future)
 */
export const layout = {
  header: {
    height: "24px", // Top header bar
    fontSize: "sm",
    py: 0.5,
  },
  sidebar: {
    width: { base: "3xs", md: "2xs", lg: "xs" }, // Responsive width
    py: 4,
  },
  footer: {
    height: "22px", // Bottom footer bar
    fontSize: { base: "xs", md: "sm" },
  },
  breadcrumb: {
    height: 6, // Document path breadcrumb
    fontSize: "13px",
    px: 3.5,
  },
  // Reserved for future security features
  lockIndicator: {
    height: "28px", // Document lock/password status indicator
  },
} as const;

export type Layout = typeof layout;
