// file: src/pages/Library.helpers.test.ts
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a

import { describe, it, expect } from 'vitest';

interface ApiImportPath {
  id: number;
  path: string;
  book_count: number;
}

interface ImportPath {
  id: number;
  path: string;
  status: 'idle' | 'scanning';
  book_count: number;
}

const toImportPaths = (folders: ApiImportPath[]): ImportPath[] =>
  folders.map((folder) => ({
    id: folder.id,
    path: folder.path,
    status: 'idle',
    book_count: folder.book_count,
  }));

describe('Library import path helpers', () => {
  it('converts API import paths to UI model', () => {
    const apiPaths: ApiImportPath[] = [
      { id: 1, path: '/a', book_count: 2 },
      { id: 2, path: '/b', book_count: 0 },
    ];

    const result = toImportPaths(apiPaths);

    expect(result).toEqual([
      { id: 1, path: '/a', status: 'idle', book_count: 2 },
      { id: 2, path: '/b', status: 'idle', book_count: 0 },
    ]);
  });
});
