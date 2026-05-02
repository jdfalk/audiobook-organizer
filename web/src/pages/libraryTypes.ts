// file: web/src/pages/libraryTypes.ts
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-11

import type { Audiobook } from '../types';

export interface ImportPath {
  id: number;
  path: string;
  status: 'idle' | 'scanning';
  book_count: number;
}

export interface BulkActionResult {
  book_id: string;
  title: string;
  status: 'updated' | 'organized' | 'error' | 'skipped';
  message?: string;
}

export interface BulkActionProgress {
  total: number;
  completed: number;
  results: BulkActionResult[];
}

export type DuplicateAction = 'skip' | 'link' | 'replace';

export type DuplicateDialogState = {
  duplicate: Audiobook;
  existing: Audiobook;
};

export type OrganizeErrorState = {
  book: Audiobook;
  message: string;
};

export const getResultLabel = (result: BulkActionResult): string => {
  if (result.message) return result.message;
  if (result.status === 'organized') return 'Organized';
  if (result.status === 'updated') return 'Updated';
  if (result.status === 'skipped') return 'Skipped';
  return 'Failed';
};
