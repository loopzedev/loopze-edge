# Migrate UI Components to Radix Vue

## Motivation

Our UI components (`FormSelect`, `FormCheckbox`, `ToggleGroup`, etc.) are based on native HTML elements with Tailwind styling. That works, but has drawbacks:

- **No accessibility** — keyboard navigation, ARIA attributes, focus management are missing
- **Inconsistent behavior** — a native `<select>` looks different on every OS and is hard to style
- **A lot of in-house development** — dropdowns with search, dialogs, tooltips would all have to be built from scratch

Radix Vue (`radix-vue@1.9.17`) is already installed as a dependency but is not used. It provides **headless UI primitives** — invisible components with correct behavior (keyboard, ARIA, focus) that we style with our Tailwind theme.

## Scope

Migrate all of our UI components under `frontend/src/components/ui/` to Radix Vue primitives.

## Component Mapping

### Migrate Immediately (already exist as our own components)

| Our component | Radix primitive | Benefit |
|---|---|---|
| `FormSelect.vue` | `SelectRoot/Trigger/Content/Item` | Fully styleable, keyboard nav, search possible |
| `FormCheckbox.vue` | `CheckboxRoot/Indicator` | Consistent rendering, ARIA, indeterminate state |
| `ToggleGroup.vue` | `ToggleGroupRoot/Item` | ARIA toggle-group, roving focus |
| `IconButton.vue` | `Toggle` (for ON/OFF) | ARIA pressed state |
| `PanelHeader.vue` | Stays (no matching primitive) | — |
| `FormLabel.vue` | `Label` | `for` attribute automatic, accessibility |
| `FormInput.vue` | Stays (native input is enough) | — |
| `SectionHeader.vue` | `Collapsible` | Sections collapsible |

### New Components (enabled by Radix)

| Component | Radix primitive | Use |
|---|---|---|
| `Tooltip.vue` | `TooltipRoot/Trigger/Content` | Node buttons, toolbar icons, handle hover |
| `Dialog.vue` | `DialogRoot/Trigger/Content/Close` | Import/export, confirmations ("Really delete?") |
| `DropdownMenu.vue` | `DropdownMenuRoot/Trigger/Content/Item` | Node right-click context menu |
| `Popover.vue` | `PopoverRoot/Trigger/Content` | Node inline config, quick edit |
| `Tabs.vue` | `TabsRoot/List/Trigger/Content` | Flow tabs (phase 4), settings tabs |
| `Switch.vue` | `SwitchRoot/Thumb` | Debug ON/OFF toggle on the node |
| `Separator.vue` | `Separator` | Consistent dividers in panels |
| `ScrollArea.vue` | `ScrollAreaRoot/Viewport/Scrollbar` | Custom scrollbars in panels |

## Implementation per Component

### `FormSelect.vue` (highest priority)

Currently: native `<select>` — cannot be styled, looks different on every OS.

New with Radix:
```vue
<script setup>
import { SelectRoot, SelectTrigger, SelectValue, SelectContent, SelectItem } from 'radix-vue'
</script>

<template>
  <SelectRoot v-model="modelValue">
    <SelectTrigger class="bg-terminal-bg border border-terminal-border text-[10px] px-2 py-1 ...">
      <SelectValue :placeholder="placeholder" />
    </SelectTrigger>
    <SelectContent class="bg-terminal-surface border border-terminal-border shadow-lg z-50 ...">
      <SelectItem v-for="opt in options" :value="opt.value" class="px-2 py-1 text-[10px] hover:bg-accent/20 ...">
        {{ opt.label }}
      </SelectItem>
    </SelectContent>
  </SelectRoot>
</template>
```

Benefits:
- Fully styleable with Tailwind (no OS-native dropdown)
- Keyboard: arrow keys, type-ahead search, Escape
- ARIA: `role="listbox"`, `aria-selected`
- Portal: content is rendered into `<body>` (no overflow clipping in panels)

### `FormCheckbox.vue`

```vue
<CheckboxRoot v-model:checked="modelValue" class="terminal-checkbox ...">
  <CheckboxIndicator class="flex items-center justify-center">
    <CheckIcon class="w-3 h-3 text-accent" />
  </CheckboxIndicator>
</CheckboxRoot>
```

### `ToggleGroup.vue`

```vue
<ToggleGroupRoot v-model="modelValue" type="single" class="flex gap-1">
  <ToggleGroupItem v-for="opt in options" :value="opt.value"
    class="px-1.5 py-0.5 text-[10px] border data-[state=on]:bg-accent data-[state=on]:border-accent ...">
    {{ opt.label }}
  </ToggleGroupItem>
</ToggleGroupRoot>
```

### `SectionHeader.vue` → Collapsible

```vue
<CollapsibleRoot v-model:open="isOpen">
  <CollapsibleTrigger class="text-[10px] text-terminal-text-dim uppercase tracking-widest mb-2 cursor-pointer">
    {{ isOpen ? '▾' : '▸' }} {{ title }}
  </CollapsibleTrigger>
  <CollapsibleContent>
    <slot />
  </CollapsibleContent>
</CollapsibleRoot>
```

Bonus: Sections in PropertyPanel can be collapsed.

### `Tooltip.vue`

```vue
<TooltipRoot>
  <TooltipTrigger as-child>
    <slot />
  </TooltipTrigger>
  <TooltipContent class="bg-terminal-surface border border-terminal-border text-[10px] px-2 py-1 shadow-lg z-50">
    {{ text }}
  </TooltipContent>
</TooltipRoot>
```

Use: All buttons that previously only had `title=""` → real tooltips with animation.

## Styling Convention

Radix uses `data-[state=...]` attributes for states:

```css
/* Active toggle */
data-[state=on]:bg-accent data-[state=on]:border-accent

/* Open Collapsible */
data-[state=open]:rotate-90

/* Selected item */
data-[highlighted]:bg-accent/20

/* Disabled */
data-[disabled]:opacity-40 data-[disabled]:cursor-not-allowed
```

This is a perfect fit for Tailwind — no additional CSS needed.

## Migration Order

1. **FormSelect** → Radix Select (largest visual impact, native selects are ugly)
2. **FormCheckbox** → Radix Checkbox (consistent rendering)
3. **ToggleGroup** → Radix ToggleGroup (accessibility)
4. **SectionHeader** → Radix Collapsible (new functionality: collapsible)
5. **Tooltip** → New (replaces all `title=""` attributes)
6. **Switch** → New (for Debug node ON/OFF, replaces toggle button)
7. **Dialog** → New (for future confirmations/import/export)
8. **DropdownMenu** → New (for node context menu)
9. **ScrollArea** → New (custom scrollbars in panels)
10. **Tabs** → New (flow tabs in phase 4)

## Do Not Migrate

| Component | Reason |
|---|---|
| `FormInput.vue` | Native `<input>` is perfect, Radix has no input primitive |
| `FormLabel.vue` | Too simple for Radix, `<label>` is enough |
| `PanelHeader.vue` | No matching primitive |
| `CodeEditor.vue` | Monaco has its own UI system |

## Dependencies

- `radix-vue@1.9.17` — already installed
- No new dependencies needed
- Icons for the checkbox indicator: SVG inline (no icon package needed)
