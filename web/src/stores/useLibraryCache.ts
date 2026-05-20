// file: web/src/stores/useLibraryCache.ts
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

import { create } from 'zustand';
import type { Audiobook } from '../types';
import type { ImportPath } from '../pages/libraryTypes';

interface LibraryCacheEntry {
  audiobooks: Audiobook[];
  totalCount: number;
  totalPages: number;
  importPaths: ImportPath[];
  timestamp: number;
}

interface LibraryStore {
  cache: Map<string, LibraryCacheEntry>;
  getCached: (key: string, maxAgeMs?: number) => LibraryCacheEntry | null;
  setCached: (key: string, entry: Omit<LibraryCacheEntry, 'timestamp'>) => void;
  clear: () => void;
}

const CACHE_TTL_MS = 1 * 60 * 1000; // 1 minute cache

export const useLibraryCache = create<LibraryStore>((set, get) => ({
  cache: new Map(),

  getCached: (key: string, maxAgeMs = CACHE_TTL_MS) => {
    const entry = get().cache.get(key);
    if (!entry) return null;

    const age = Date.now() - entry.timestamp;
    if (age > maxAgeMs) {
      get().cache.delete(key);
      return null;
    }

    return entry;
  },

  setCached: (key: string, entry) => {
    set((state) => {
      const newCache = new Map(state.cache);
      newCache.set(key, { ...entry, timestamp: Date.now() });
      return { cache: newCache };
    });
  },

  clear: () => {
    set({ cache: new Map() });
  },
}));

export const buildCacheKey = (
  page: number,
  itemsPerPage: number,
  searchQuery: string,
  filters: string,
  sortBy: string,
  sortOrder: string
): string => {
  return `${page}:${itemsPerPage}:${searchQuery}:${filters}:${sortBy}:${sortOrder}`;
};
