# Design Notes

## Sunrise Theme Contrast

The sunrise palette is scoped to `.sunrise` on the landing page. Contrast is measured using WCAG 2.1 relative luminance.

| Foreground | Background | Ratio | Use |
| --- | --- | --- | --- |
| ink `#241810` | cream `#FFF8EC` | 16.40:1 | body / large |
| ink `#241810` | sand `#FCE7C2` | 14.30:1 | body / large |
| ink `#241810` | sun `#F6A31B` | 8.39:1 | body / large |
| cream `#FFF8EC` | leaf `#2E6B4B` | 5.99:1 | body / large |
| cream `#FFF8EC` | leaf-deep `#1F4D36` | 9.15:1 | body / large |
| terracotta `#B5482A` | cream `#FFF8EC` | 5.07:1 | body / large |
| terracotta `#B5482A` | sand `#FCE7C2` | 4.42:1 | large text / icons only |
| leaf-deep `#1F4D36` | sun `#F6A31B` | 4.68:1 | accent text |

Never place white/cream on sun (1.96:1) or ink on leaf (2.74:1).
