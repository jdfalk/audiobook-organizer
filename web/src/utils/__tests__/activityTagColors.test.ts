// file: web/src/utils/__tests__/activityTagColors.test.ts
// version: 1.1.0
// guid: f1e2d3c4-b5a6-7890-cdef-1234567890ab

import { describe, it, expect } from 'vitest';
import { tagChipProps } from '../activityTagColors';

describe('tagChipProps', () => {
  it('action: prefix → primary color, stripped label', () => {
    const p = tagChipProps('action:metadata-apply');
    expect(p.color).toBe('primary');
    expect(p.label).toBe('metadata-apply');
  });

  it('source: prefix → default color + custom sx', () => {
    const p = tagChipProps('source:pipeline');
    expect(p.color).toBe('default');
    expect(p.sx).toBeDefined();
    expect(p.label).toBe('pipeline');
  });

  it('outcome:ok → success', () => {
    expect(tagChipProps('outcome:ok').color).toBe('success');
  });

  it('outcome:warn → default color + yellow sx + "warning" label', () => {
    const p = tagChipProps('outcome:warn');
    expect(p.color).toBe('default');
    expect(p.sx).toBeDefined();
    expect(p.label).toBe('warning');
  });

  it('outcome:error → error', () => {
    expect(tagChipProps('outcome:error').color).toBe('error');
  });

  it('outcome:skip → default', () => {
    expect(tagChipProps('outcome:skip').color).toBe('default');
  });

  it('op: prefix → info color', () => {
    const p = tagChipProps('op:01J123ABC');
    expect(p.color).toBe('info');
    expect(p.label).toBe('01J123ABC');
  });

  it('book: prefix → default color + orange sx', () => {
    const p = tagChipProps('book:abc123');
    expect(p.color).toBe('default');
    expect(p.sx).toBeDefined();
    expect(p.label).toBe('abc123');
  });

  it('scope: prefix → default', () => {
    expect(tagChipProps('scope:book').color).toBe('default');
    expect(tagChipProps('scope:book').label).toBe('book');
  });

  it('lifecycle: prefix → default', () => {
    expect(tagChipProps('lifecycle:startup').color).toBe('default');
  });

  it('unknown namespace → default, full tag as label', () => {
    const p = tagChipProps('no-op');
    expect(p.color).toBe('default');
    expect(p.label).toBe('no-op');
  });

  it('unknown namespace with colon → default', () => {
    const p = tagChipProps('foo:bar');
    expect(p.color).toBe('default');
    expect(p.label).toBe('bar');
  });
});
