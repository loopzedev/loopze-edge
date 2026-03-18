# UI-Komponenten auf Radix Vue migrieren

## Motivation

Unsere UI-Komponenten (`FormSelect`, `FormCheckbox`, `ToggleGroup`, etc.) basieren auf nativen HTML-Elementen mit Tailwind-Styling. Das funktioniert, hat aber Nachteile:

- **Keine Accessibility** — Keyboard-Navigation, ARIA-Attribute, Focus-Management fehlen
- **Inkonsistentes Verhalten** — native `<select>` sieht auf jedem OS anders aus, lässt sich kaum stylen
- **Viel Eigenentwicklung** — Dropdowns mit Suche, Dialoge, Tooltips müssten komplett selbst gebaut werden

Radix Vue (`radix-vue@1.9.17`) ist bereits als Dependency installiert aber wird nicht genutzt. Es liefert **headless UI-Primitives** — unsichtbare Komponenten mit korrektem Verhalten (Keyboard, ARIA, Focus), die wir mit unserem Tailwind-Theme stylen.

## Scope

Alle unsere UI-Komponenten unter `frontend/src/components/ui/` auf Radix Vue Primitives umstellen.

## Komponenten-Mapping

### Sofort migrieren (existieren bereits als eigene Komponenten)

| Unsere Komponente | Radix Primitive | Vorteil |
|---|---|---|
| `FormSelect.vue` | `SelectRoot/Trigger/Content/Item` | Vollständig stylebar, Keyboard-Nav, Suchfunktion möglich |
| `FormCheckbox.vue` | `CheckboxRoot/Indicator` | Konsistentes Rendering, ARIA, Indeterminate-State |
| `ToggleGroup.vue` | `ToggleGroupRoot/Item` | ARIA toggle-group, roving focus |
| `IconButton.vue` | `Toggle` (für ON/OFF) | ARIA pressed state |
| `PanelHeader.vue` | Bleibt (kein passendes Primitive) | — |
| `FormLabel.vue` | `Label` | `for`-Attribut automatisch, Accessibility |
| `FormInput.vue` | Bleibt (natives Input reicht) | — |
| `SectionHeader.vue` | `Collapsible` | Sections ein-/ausklappbar |

### Neue Komponenten (ermöglicht durch Radix)

| Komponente | Radix Primitive | Einsatz |
|---|---|---|
| `Tooltip.vue` | `TooltipRoot/Trigger/Content` | Node-Buttons, Toolbar-Icons, Handle-Hover |
| `Dialog.vue` | `DialogRoot/Trigger/Content/Close` | Import/Export, Bestätigungen ("Wirklich löschen?") |
| `DropdownMenu.vue` | `DropdownMenuRoot/Trigger/Content/Item` | Node-Rechtsklick-Kontextmenü |
| `Popover.vue` | `PopoverRoot/Trigger/Content` | Node-Inline-Config, Quick-Edit |
| `Tabs.vue` | `TabsRoot/List/Trigger/Content` | Flow-Tabs (Phase 4), Settings-Tabs |
| `Switch.vue` | `SwitchRoot/Thumb` | Debug ON/OFF Toggle am Node |
| `Separator.vue` | `Separator` | Konsistente Trennlinien in Panels |
| `ScrollArea.vue` | `ScrollAreaRoot/Viewport/Scrollbar` | Custom-Scrollbars in Panels |

## Implementierung pro Komponente

### `FormSelect.vue` (höchste Priorität)

Aktuell: natives `<select>` — kann nicht gestyled werden, sieht auf jedem OS anders aus.

Neu mit Radix:
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

Vorteile:
- Vollständig mit Tailwind stylebar (kein OS-natives Dropdown)
- Keyboard: Arrow Keys, Type-ahead Suche, Escape
- ARIA: `role="listbox"`, `aria-selected`
- Portal: Content wird in `<body>` gerendert (kein Overflow-Clipping in Panels)

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

Bonus: Sections in PropertyPanel können ein-/ausgeklappt werden.

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

Einsatz: Alle Buttons die bisher nur `title=""` haben → echte Tooltips mit Animation.

## Styling-Konvention

Radix verwendet `data-[state=...]` Attribute für States:

```css
/* Aktiver Toggle */
data-[state=on]:bg-accent data-[state=on]:border-accent

/* Geöffneter Collapsible */
data-[state=open]:rotate-90

/* Selektiertes Item */
data-[highlighted]:bg-accent/20

/* Disabled */
data-[disabled]:opacity-40 data-[disabled]:cursor-not-allowed
```

Das passt perfekt zu Tailwind — kein zusätzliches CSS nötig.

## Migrationsreihenfolge

1. **FormSelect** → Radix Select (größter visueller Impact, native Selects sind hässlich)
2. **FormCheckbox** → Radix Checkbox (konsistentes Rendering)
3. **ToggleGroup** → Radix ToggleGroup (Accessibility)
4. **SectionHeader** → Radix Collapsible (neue Funktionalität: ein-/ausklappbar)
5. **Tooltip** → Neu (ersetzt alle `title=""` Attribute)
6. **Switch** → Neu (für Debug Node ON/OFF, ersetzt Toggle-Button)
7. **Dialog** → Neu (für zukünftige Bestätigungen/Import/Export)
8. **DropdownMenu** → Neu (für Node-Kontextmenü)
9. **ScrollArea** → Neu (Custom-Scrollbars in Panels)
10. **Tabs** → Neu (Flow-Tabs in Phase 4)

## Nicht migrieren

| Komponente | Grund |
|---|---|
| `FormInput.vue` | Natives `<input>` ist perfekt, Radix hat kein Input-Primitive |
| `FormLabel.vue` | Zu simpel für Radix, `<label>` reicht |
| `PanelHeader.vue` | Kein passendes Primitive |
| `CodeEditor.vue` | Monaco hat eigenes UI-System |

## Abhängigkeiten

- `radix-vue@1.9.17` — bereits installiert
- Keine neuen Dependencies nötig
- Icons für Checkbox-Indicator: SVG inline (kein Icon-Package nötig)
