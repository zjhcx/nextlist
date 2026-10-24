import { createEffect, For, onCleanup } from "solid-js"
import { local, setLocal } from "~/store/local_settings"
import { ACCENT_PRESETS, accentPaletteStyleVars } from "~/store/accent"

const defaults = {
  background: "",
  theme: "",
  card_opacity: "100",
  component_opacity: "100",
  blur: "0",
}
type Name = keyof typeof defaults
const key = (name: string) => `appearance_${name}`
const value = (name: Name) => local[key(name)] ?? defaults[name]
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
        if (base)
          vars[k] = `color-mix(in srgb, ${base} ${alpha}%, transparent)`
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
export const appearancePaletteDeps = () => [siteThemeColor(), componentOpacity()]

export function AppearanceSettings() {
  return (
    <fieldset class="appearance-settings">
      <legend>外观 · Appearance</legend>
      <p>设置自动保存在当前浏览器，清空图片地址可恢复纯灰背景。</p>

      <label>
        主题色 / Theme color <output>{siteThemeColor() || "默认 / Default"}</output>
      </label>
      <div class="appearance-swatches">
        <For each={ACCENT_PRESETS}>
          {(preset) => (
            <button
              type="button"
              class="appearance-swatch"
              title={preset.name}
              aria-label={preset.name}
              aria-pressed={siteThemeColor() === preset.color}
              style={{ "background-color": preset.color }}
              onClick={() => setLocal(key("theme"), preset.color)}
            />
          )}
        </For>
        <input
          type="color"
          aria-label="Custom theme color"
          value={siteThemeColor() || "#2563eb"}
          onInput={(e) => setLocal(key("theme"), e.currentTarget.value)}
        />
      </div>

      <label for="appearance-background">背景图片 URL / Image URL</label>
      <input
        id="appearance-background"
        type="url"
        placeholder="https://…"
        value={value("background")}
        onInput={(e) => setLocal(key("background"), e.currentTarget.value)}
      />
      <label for="appearance-card-opacity">
        卡片透明度 / Card opacity <output>{cardOpacity()}%</output>
      </label>
      <input
        id="appearance-card-opacity"
        type="range"
        min="30"
        max="100"
        value={cardOpacity()}
        onInput={(e) => setLocal(key("card_opacity"), e.currentTarget.value)}
      />
      <label for="appearance-component-opacity">
        组件透明度 / Component opacity <output>{componentOpacity()}%</output>
      </label>
      <input
        id="appearance-component-opacity"
        type="range"
        min="30"
        max="100"
        value={componentOpacity()}
        onInput={(e) =>
          setLocal(key("component_opacity"), e.currentTarget.value)
        }
      />
      <label for="appearance-blur">
        磨砂模糊 / Blur <output>{number("blur", 0, 32)}px</output>
      </label>
      <input
        id="appearance-blur"
        type="range"
        min="0"
        max="32"
        value={number("blur", 0, 32)}
        onInput={(e) => setLocal(key("blur"), e.currentTarget.value)}
      />
      <button
        type="button"
        onClick={() => {
          for (const [name, initial] of Object.entries(defaults))
            setLocal(key(name), initial)
        }}
      >
        恢复默认 / Reset
      </button>
    </fieldset>
  )
}
