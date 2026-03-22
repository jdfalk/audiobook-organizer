// file: web/src/hooks/useColumnConfig.ts
// version: 1.0.0
// guid: b3c4d5e6-f7a8-49b0-c1d2-e3f4a5b6c7d8

import { useState, useEffect, useCallback, useRef } from 'react';
import {
  type ColumnConfig,
  getUserColumnConfig,
  saveUserColumnConfig,
  deleteUserColumnConfig,
} from '../services/api';
import {
  type ColumnDefinition,
  ALL_COLUMNS,
  COLUMN_MAP,
  getDefaultVisibleColumns,
} from '../config/columnDefinitions';

export interface UseColumnConfigReturn {
  columns: ColumnDefinition[]; // visible columns in order, with widths applied
  visibleColumnIds: string[];
  columnWidths: Record<string, number>;
  isLoading: boolean;
  toggleColumn: (columnId: string) => void;
  reorderColumns: (columnIds: string[]) => void;
  resizeColumn: (columnId: string, width: number) => void;
  resetToDefaults: () => void;
}

const DEBOUNCE_MS = 500;

function buildDefaults(): ColumnConfig {
  const defaultCols = getDefaultVisibleColumns();
  return {
    visibleColumns: defaultCols.map((c) => c.id),
    columnOrder: defaultCols.map((c) => c.id),
    columnWidths: {},
  };
}

function resolveColumns(config: ColumnConfig): ColumnDefinition[] {
  const widths = config.columnWidths;
  const visible = new Set(config.visibleColumns);

  return config.columnOrder
    .filter((id) => visible.has(id) && COLUMN_MAP.has(id))
    .map((id) => {
      const def = COLUMN_MAP.get(id)!;
      const width = widths[id];
      if (width != null && width !== def.defaultWidth) {
        return { ...def, defaultWidth: width };
      }
      return def;
    });
}

export function useColumnConfig(): UseColumnConfigReturn {
  const [config, setConfig] = useState<ColumnConfig>(buildDefaults);
  const [isLoading, setIsLoading] = useState(true);
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isMountedRef = useRef(true);

  // Load saved config on mount
  useEffect(() => {
    isMountedRef.current = true;
    let cancelled = false;

    getUserColumnConfig()
      .then((saved) => {
        if (cancelled) return;
        if (saved) {
          // Validate that column IDs still exist
          const validVisible = saved.visibleColumns.filter((id) =>
            COLUMN_MAP.has(id)
          );
          const validOrder = saved.columnOrder.filter((id) =>
            COLUMN_MAP.has(id)
          );
          // Add any visible columns not in order
          const orderSet = new Set(validOrder);
          for (const id of validVisible) {
            if (!orderSet.has(id)) validOrder.push(id);
          }
          setConfig({
            visibleColumns: validVisible,
            columnOrder: validOrder,
            columnWidths: saved.columnWidths ?? {},
          });
        }
      })
      .catch(() => {
        // Use defaults on error — already set
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });

    return () => {
      cancelled = true;
      isMountedRef.current = false;
    };
  }, []);

  // Debounced save
  const scheduleSave = useCallback((newConfig: ColumnConfig) => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      saveUserColumnConfig(newConfig).catch(() => {
        // Silently ignore save errors — config is still in local state
      });
    }, DEBOUNCE_MS);
  }, []);

  // Cleanup timer on unmount
  useEffect(() => {
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    };
  }, []);

  const toggleColumn = useCallback(
    (columnId: string) => {
      if (!COLUMN_MAP.has(columnId)) return;
      setConfig((prev) => {
        const isVisible = prev.visibleColumns.includes(columnId);
        let nextVisible: string[];
        let nextOrder: string[];

        if (isVisible) {
          // Hide: remove from visible and order
          nextVisible = prev.visibleColumns.filter((id) => id !== columnId);
          nextOrder = prev.columnOrder.filter((id) => id !== columnId);
        } else {
          // Show: append to end
          nextVisible = [...prev.visibleColumns, columnId];
          nextOrder = [...prev.columnOrder, columnId];
        }

        const next: ColumnConfig = {
          visibleColumns: nextVisible,
          columnOrder: nextOrder,
          columnWidths: prev.columnWidths,
        };
        scheduleSave(next);
        return next;
      });
    },
    [scheduleSave]
  );

  const reorderColumns = useCallback(
    (columnIds: string[]) => {
      setConfig((prev) => {
        const next: ColumnConfig = {
          visibleColumns: prev.visibleColumns,
          columnOrder: columnIds,
          columnWidths: prev.columnWidths,
        };
        scheduleSave(next);
        return next;
      });
    },
    [scheduleSave]
  );

  const resizeColumn = useCallback(
    (columnId: string, width: number) => {
      setConfig((prev) => {
        const next: ColumnConfig = {
          visibleColumns: prev.visibleColumns,
          columnOrder: prev.columnOrder,
          columnWidths: { ...prev.columnWidths, [columnId]: width },
        };
        scheduleSave(next);
        return next;
      });
    },
    [scheduleSave]
  );

  const resetToDefaults = useCallback(() => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    const defaults = buildDefaults();
    setConfig(defaults);
    deleteUserColumnConfig().catch(() => {
      // Silently ignore
    });
  }, []);

  const columns = resolveColumns(config);

  return {
    columns,
    visibleColumnIds: config.visibleColumns,
    columnWidths: config.columnWidths,
    isLoading,
    toggleColumn,
    reorderColumns,
    resizeColumn,
    resetToDefaults,
  };
}
