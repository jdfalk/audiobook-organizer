// file: web/src/utils/activityTagColors.ts
// version: 1.2.0
// guid: c1d2e3f4-a5b6-7c8d-9e0f-1a2b3c4d5e6f

/**
 * tagChipProps returns MUI Chip color+sx props for a given activity tag.
 * Namespace prefixes set the chip color; the label strips the namespace so
 * "action:metadata-apply" renders as "metadata-apply" in a blue chip.
 *
 * Namespaces:
 *   action:*     → primary (blue)
 *   source:*     → secondary (purple)
 *   outcome:ok   → success (green)
 *   outcome:warn → yellow
 *   outcome:error→ error (red)
 *   outcome:skip → default (gray)
 *   op:*         → info (teal)
 *   book:*       → orange (custom sx)
 *   component:*  → secondary-light (indigo-ish custom sx)
 *   scope:*      → default (gray)
 *   lifecycle:*  → default (gray)
 *   anything else→ default (gray)
 */
export type ChipColor =
  | 'default'
  | 'primary'
  | 'secondary'
  | 'error'
  | 'info'
  | 'success'
  | 'warning';

export interface TagChipProps {
  color: ChipColor;
  sx?: Record<string, unknown>;
  label: string;
}

export function tagChipProps(tag: string): TagChipProps {
  const colonIdx = tag.indexOf(':');
  const ns = colonIdx > 0 ? tag.slice(0, colonIdx) : '';
  const val = colonIdx > 0 ? tag.slice(colonIdx + 1) : tag;

  switch (ns) {
    case 'action':
      return { color: 'primary', label: val };
    case 'source':
      return { color: 'default', sx: { borderColor: '#78909c', color: '#cfd8dc' }, label: val };
    case 'outcome':
      switch (val) {
        case 'ok':
          return { color: 'success', label: val };
        case 'warn':
          return {
            color: 'default',
            sx: { borderColor: '#fdd835', color: '#fff176', bgcolor: 'rgba(253, 216, 53, 0.08)' },
            label: 'warning',
          };
        case 'error':
          return { color: 'error', label: val };
        case 'skip':
          return { color: 'default', label: val };
        default:
          return { color: 'default', label: val };
      }
    case 'op':
      return { color: 'info', label: val };
    case 'book':
      return {
        color: 'default',
        sx: { bgcolor: '#ffb74d', color: '#000' },
        label: val,
      };
    case 'component':
      return {
        color: 'default',
        sx: { bgcolor: '#7986cb', color: '#fff' },
        label: val,
      };
    case 'plugin':
      return { color: 'secondary', label: val };
    case 'def':
      return {
        color: 'default',
        sx: { borderColor: '#64b5f6', color: '#90caf9' },
        label: val,
      };
    case 'phase':
      return {
        color: 'default',
        sx: { borderColor: '#4db6ac', color: '#80cbc4' },
        label: val,
      };
    case 'http':
      return {
        color: 'default',
        sx: { borderColor: '#90a4ae', color: '#cfd8dc' },
        label: val,
      };
    case 'domain':
      return { color: 'primary', label: val };
    case 'network':
      return {
        color: 'default',
        sx: { borderColor: '#81d4fa', color: '#b3e5fc' },
        label: val,
      };
    case 'error':
      return { color: 'error', label: val };
    case 'scope':
    case 'lifecycle':
      return { color: 'default', label: val };
    default:
      // For any other namespace:value tag, strip the namespace so the chip
      // shows the meaningful value (e.g. "no-op" stays as-is since there's
      // no colon, but "foo:bar" shows "bar").
      return { color: 'default', label: ns ? val : tag };
  }
}
