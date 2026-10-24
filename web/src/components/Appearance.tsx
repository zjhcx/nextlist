import { createEffect, onCleanup } from "solid-js"
import { getSetting } from "~/store/settings"
import { accentPaletteStyleVars } from "~/store/accent"

const defaults = {
  background: "",
  theme: "",
  card_opacity: "100",
  component_opacity: "100",
  blur: "0",
}
type Name = keyof typeof defaults
const key = (name: string) => `appearance_${name}`
const value = (name: Name) => getSetting(key(name)) || defaults[name]
const number = (
  name: "card_opacity" | "component_opacity" | "blur",
  min: number,
  max: number,
) => {
  const parsed = Number(value(name))
  return Number.isFinite(parsed)
    ? Math.max(min, Math.min(max, parsed))
    : Number(defaults[name])
}
const cardOpacity = () => number("card_opacity", 30, 100)
const componentOpacity = () => number("component_opacity", 30, 100)

/** Site theme color chosen in appearance settings ("" = site default). */
export const siteThemeColor = () => {
  const color = value("theme").trim()
  return /^#[0-9a-f]{6}$/i.test(color) ? color : ""
}

// Serialize a parsed image URL rather than accepting arbitrary CSS.
function backgroundImage() {
  try {
    const input = value("background").trim()
    const url = new URL(input, window.location.href)
    return input && ["http:", "https:"].includes(url.protocol)
      ? `url(${JSON.stringify(url.href)})`
      : "none"
  } catch {
    return "none"
  }
}

// ==================== palette ====================

const SCALES = [
  "neutral",
  "primary",
  "accent",
  "info",
  "success",
  "warning",
  "danger",
]
// Background steps used by filled / subtle components (normal, hover, active).
const COMPONENT_STEPS = [3, 4, 5, 6]
const MANAGED_KEYS = SCALES.flatMap((scale) =>
  Array.from({ length: 12 }, (_, i) => `--hope-colors-${scale}${i + 1}`),
)

/**
 * Owns every inline `--hope-colors-*` override: theme color palette first,
 * then component transparency on the background steps.
 */
export const applyAppearancePalette = (theme: string, dark: boolean) => {
  if (typeof document === "undefined") return
  const targets = [document.documentElement, document.body]
  targets.forEach((el) =>
    MANAGED_KEYS.forEach((k) => el.style.removeProperty(k)),
  )
  const vars: Record<string, string> = theme
    ? accentPaletteStyleVars(theme, dark)
    : {}
  const alpha = componentOpacity()
  if (alpha < 100) {
    const computed = getComputedStyle(document.body)
    SCALES.forEach((scale) =>
      COMPONENT_STEPS.forEach((step) => {
        const k = `--hope-colors-${scale}${step}`
        const base = vars[k] ?? computed.getPropertyValue(k).trim()
        if (base) vars[k] = `color-mix(in srgb, ${base} ${alpha}%, transparent)`
      }),
    )
  }
  targets.forEach((el) =>
    Object.entries(vars).forEach(([k, v]) => el.style.setProperty(k, v)),
  )
}

export function AppearanceBackground() {
  createEffect(() => {
    const root = document.documentElement.style
    root.setProperty("--card-opacity", `${cardOpacity()}%`)
    root.setProperty("--component-opacity", `${componentOpacity()}%`)
    root.setProperty("--glass-blur", `${number("blur", 0, 32)}px`)
  })
  onCleanup(() => {
    const root = document.documentElement.style
    root.removeProperty("--card-opacity")
    root.removeProperty("--component-opacity")
    root.removeProperty("--glass-blur")
  })
  return (
    <div
      class="appearance-background"
      aria-hidden="true"
      style={{ "background-image": backgroundImage() }}
    />
  )
}

/** Reactive dependency helper so palette effects rerun on changes. */
export const appearancePaletteDeps = () => [
  siteThemeColor(),
  componentOpacity(),
]
