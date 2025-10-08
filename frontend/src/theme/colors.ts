/**
 * Centralized color palette for dark and light modes.
 *
 * Usage:
 * - Import: import { colors } from './theme/colors'
 * - Use conditionally: bgColor={darkMode ? colors.dark.bg.primary : colors.light.bg.primary}
 *
 * ⚠️ Important:
 * - Keep hex values here, not scattered in components
 * - When adding new colors, add to both dark and light modes
 * - Use semantic names (primary/secondary/tertiary) not descriptive (darkest/lighter)
 */
export const colors = {
  dark: {
    bg: {
      primary: "#1e1e1e", // Main background
      secondary: "#252526", // Sidebar background
      tertiary: "#333333", // Card/popover background
      elevated: "#3c3c3c", // Input/select background
      hover: "#464647", // Hover state
      footer: "#00A6ED", // Footer blue
      footerAccent: "#FFB400", // Footer yellow/gold section
      footerAccentHover: "#FFC933", // Footer yellow/gold hover
    },
    border: "#464647",
    text: {
      primary: "#cbcaca", // Main text
      secondary: "#cccccc", // Secondary text
      muted: "#888888", // Breadcrumb, subtle text
      header: "#cccccc", // Header text
      footer: "#000000", // Footer text
    },
    accent: {
      documentIcon: "#F6511D", // Orange/coral document icon
      folderIcon: "#00A6ED", // Folder icon
      link: "#00A6ED", // Link color
    },
  },
  light: {
    bg: {
      primary: "white", // Main background
      secondary: "#f3f3f3", // Sidebar background
      tertiary: "#e8e8e8", // Header background
      elevated: "white", // Input/select background
      hover: "gray.200", // Hover state
      footer: "#00A6ED", // Footer blue
      footerAccent: "#FFB400", // Footer yellow/gold section
      footerAccentHover: "#FFC933", // Footer yellow/gold hover
    },
    border: "gray.200",
    text: {
      primary: "inherit", // Main text (use browser default)
      secondary: "#383838", // Header text
      muted: "#888888", // Breadcrumb, subtle text
      footer: "#000000", // Footer text
    },
    accent: {
      documentIcon: "#F6511D", // Orange/coral document icon
      folderIcon: "#00A6ED", // Folder icon
      link: "#00A6ED", // Link color
    },
  },
} as const;

export type Colors = typeof colors;
