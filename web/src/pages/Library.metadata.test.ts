// file: web/src/pages/Library.metadata.test.ts
// version: 1.0.0
// guid: 2d2b25f0-5f4f-4a06-9d87-2f93a02d6e0f

import { describe, it, expect } from 'vitest';
import { buildMetadataUpdatePayload } from './Library.metadata';
import type { Audiobook } from '../types';

const baseAudiobook: Audiobook = {
  id: 'book-1',
  title: 'Test Title',
  file_path: '/tmp/book.m4b',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('buildMetadataUpdatePayload', () => {
  it('maps author, series, and year fields', () => {
    const payload = buildMetadataUpdatePayload({
      ...baseAudiobook,
      author: 'Test Author',
      series: 'Series Name',
      series_number: 2,
      year: 2020,
      narrator: 'Narrator',
      publisher: 'Publisher',
    });

    expect(payload.author_name).toBe('Test Author');
    expect(payload.series_name).toBe('Series Name');
    expect(payload.series_position).toBe(2);
    expect(payload.audiobook_release_year).toBe(2020);
    expect(payload.narrator).toBe('Narrator');
    expect(payload.publisher).toBe('Publisher');
  });

  it('omits blank strings and zero values', () => {
    const payload = buildMetadataUpdatePayload({
      ...baseAudiobook,
      author: '   ',
      series: '',
      series_number: 0,
      year: 0,
      language: '  ',
    });

    expect(payload.author_name).toBeUndefined();
    expect(payload.series_name).toBeUndefined();
    expect(payload.series_position).toBeUndefined();
    expect(payload.audiobook_release_year).toBeUndefined();
    expect(payload.language).toBeUndefined();
  });
});
