// file: web/src/pages/Library.metadata.ts
// version: 1.0.0
// guid: 6b1d8e62-9f3d-4b9a-9db8-25d5d7f5d0ed

import type { Audiobook } from '../types';
import type { Book } from '../services/api';

const normalizeString = (value?: string): string | undefined => {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
};

const normalizeNumber = (value?: number): number | undefined => {
  if (value === undefined || value === null) return undefined;
  return value > 0 ? value : undefined;
};

export const buildMetadataUpdatePayload = (
  audiobook: Audiobook
): Partial<Book> & {
  author_name?: string;
  series_name?: string;
  series_position?: number;
  audiobook_release_year?: number;
} => {
  const releaseYear =
    audiobook.audiobook_release_year ?? audiobook.year ?? undefined;

  return {
    title: audiobook.title,
    narrator: normalizeString(audiobook.narrator),
    publisher: normalizeString(audiobook.publisher),
    language: normalizeString(audiobook.language),
    edition: normalizeString(audiobook.edition),
    isbn10: normalizeString(audiobook.isbn10),
    isbn13: normalizeString(audiobook.isbn13),
    author_name: normalizeString(audiobook.author),
    series_name: normalizeString(audiobook.series),
    series_position: normalizeNumber(audiobook.series_number),
    audiobook_release_year: normalizeNumber(releaseYear),
  };
};
