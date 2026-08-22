import type { Appearance } from "@stripe/stripe-js";

/** Matches `app/globals.css` tokens used by the rest of the site. */
export const sponsorCheckoutAppearance: Appearance = {
  theme: "night",
  labels: "above",
  inputs: "spaced",
  variables: {
    colorPrimary: "#8b5cf6",
    colorBackground: "#0a0a0a",
    colorText: "#e5e5e5",
    colorTextSecondary: "#737373",
    colorTextPlaceholder: "#737373",
    colorDanger: "#f87171",
    colorSuccess: "#4ade80",
    accessibleColorOnColorPrimary: "#e5e5e5",
    accessibleColorOnColorBackground: "#e5e5e5",
    borderRadius: "0px",
    fontFamily: "system-ui, sans-serif",
    fontSizeBase: "14px",
    spacingUnit: "4px",
    inputColorBorder: "#262626",
    inputFocusColorBorder: "#8b5cf6",
    inputBoxShadow: "none",
    inputFocusBoxShadow: "none",
    focusBoxShadow: "none",
    focusOutline: "none",
    tabLogoColor: "light",
    tabLogoSelectedColor: "light",
    blockLogoColor: "light",
    logoColor: "light",
    iconColor: "#737373",
    iconHoverColor: "#e5e5e5",
  },
  rules: {
    ".Input": {
      backgroundColor: "#0a0a0a",
      border: "1px solid #262626",
      boxShadow: "none",
      color: "#e5e5e5",
      padding: "8px 12px",
    },
    ".Input:focus": {
      border: "1px solid #8b5cf6",
      boxShadow: "none",
    },
    ".Label": {
      color: "#737373",
    },
    ".Tab": {
      backgroundColor: "#0a0a0a",
      border: "1px solid #262626",
      boxShadow: "none",
      color: "#737373",
    },
    ".Tab:hover": {
      color: "#e5e5e5",
    },
    ".Tab--selected": {
      backgroundColor: "#16111f",
      border: "1px solid #8b5cf6",
      color: "#e5e5e5",
    },
    ".AccordionItem": {
      backgroundColor: "#0a0a0a",
      border: "1px solid #262626",
      boxShadow: "none",
    },
    ".Block": {
      backgroundColor: "#0a0a0a",
      border: "1px solid #262626",
      boxShadow: "none",
    },
  },
};
