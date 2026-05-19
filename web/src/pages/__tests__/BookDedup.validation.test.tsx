/**
 * BookDedup.validation.test.tsx
 * @version 1.0.0
 *
 * Tests for ULID validation in AcousticComparePanel
 */

import { describe, it, expect } from 'vitest';

/**
 * Standalone validation function test
 * This mirrors the validateBookID function in AcousticComparePanel
 */
const validateBookID = (id: string): string | null => {
  const ulidPattern = /^[0-9A-Z]{26}$/;
  const trimmed = id.trim();
  if (!trimmed) return 'Book ID is required';
  if (!ulidPattern.test(trimmed)) {
    return 'Invalid book ID format. Must be 26-character alphanumeric (0-9, A-Z only).';
  }
  return null;
};

describe('BookDedup validation', () => {
  describe('validateBookID function', () => {
    const validULIDs = [
      '01ARZ3NDEKTSV4RRFFQ69G5FAV',
      '01ARZ3NDEKTSV4RRFFQ69G5FAW',
      '0000000000000000000000000A',
      'ZZZZZZZZZZZZZZZZZZZZZZZZZZ',
      '0123456789ABCDEFGHIJKLMNOP',
    ];

    const invalidULIDs = [
      { value: '01ARZ3NDEKTSV4RRFFQ69G5FA', reason: 'too short (25 chars)' },
      { value: '01ARZ3NDEKTSV4RRFFQ69G5FAVA', reason: 'too long (27 chars)' },
      { value: '01ARZ3NDEKTSV4RRFFQ69G5Fav', reason: 'lowercase letters' },
      { value: '01ARZ3NDEKTSV4RRFFQ69G-FAV', reason: 'invalid character (-)' },
      { value: '01ARZ3NDEKTSV4RRFFQ69G FAV', reason: 'space character' },
      { value: '', reason: 'empty string' },
      { value: '   ', reason: 'whitespace only' },
    ];

    describe('valid ULIDs', () => {
      validULIDs.forEach((ulid) => {
        it(`should accept valid ULID: ${ulid}`, () => {
          const result = validateBookID(ulid);
          expect(result).toBeNull();
        });
      });

      validULIDs.forEach((ulid) => {
        it(`should accept valid ULID with leading/trailing spaces: "  ${ulid}  "`, () => {
          const result = validateBookID(`  ${ulid}  `);
          expect(result).toBeNull();
        });
      });
    });

    describe('invalid ULIDs', () => {
      invalidULIDs.forEach(({ value, reason }) => {
        it(`should reject ULID that is ${reason}: "${value}"`, () => {
          const result = validateBookID(value);
          expect(result).not.toBeNull();
        });
      });
    });

    describe('error messages', () => {
      it('should return "Book ID is required" for empty string', () => {
        const result = validateBookID('');
        expect(result).toBe('Book ID is required');
      });

      it('should return "Book ID is required" for whitespace only', () => {
        const result = validateBookID('   ');
        expect(result).toBe('Book ID is required');
      });

      it('should return format error for invalid ULID', () => {
        const result = validateBookID('invalid');
        expect(result).toContain('Invalid book ID format');
        expect(result).toContain('26-character');
      });

      it('should return format error for lowercase ULID', () => {
        const result = validateBookID('01arz3ndektsv4rrffq69g5fav');
        expect(result).toContain('Invalid book ID format');
      });
    });

    describe('edge cases', () => {
      it('should handle exactly 26 characters', () => {
        const ulid = 'A'.repeat(26);
        expect(validateBookID(ulid)).toBeNull();
      });

      it('should reject 25 characters', () => {
        const ulid = 'A'.repeat(25);
        expect(validateBookID(ulid)).not.toBeNull();
      });

      it('should reject 27 characters', () => {
        const ulid = 'A'.repeat(27);
        expect(validateBookID(ulid)).not.toBeNull();
      });

      it('should allow mix of numbers and uppercase letters', () => {
        const ulid = '0123456789ABCDEFGHIJKLMNOP';
        expect(validateBookID(ulid)).toBeNull();
      });

      it('should reject mix of numbers and lowercase letters', () => {
        const ulid = '0123456789abcdefghijklmnop';
        expect(validateBookID(ulid)).not.toBeNull();
      });

      it('should reject ULID with special characters', () => {
        const specialChars = ['!', '@', '#', '$', '%', '^', '&', '*', '(', ')'];
        specialChars.forEach((char) => {
          const ulid = `0123456789ABCDEFGHIJKLMN${char}P`;
          expect(validateBookID(ulid)).not.toBeNull();
        });
      });
    });
  });
});
