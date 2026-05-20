// file: web/src/hooks/useTimeout.ts
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

import { useEffect, useRef } from 'react';

/**
 * Safe setTimeout hook that automatically cleans up on unmount.
 * Prevents setState on unmounted components.
 */
export function useTimeout(callback: () => void, delay: number) {
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isUnmountedRef = useRef(false);

  useEffect(() => {
    isUnmountedRef.current = false;
    timeoutRef.current = setTimeout(() => {
      if (!isUnmountedRef.current) {
        callback();
      }
      timeoutRef.current = null;
    }, delay);

    return () => {
      isUnmountedRef.current = true;
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
        timeoutRef.current = null;
      }
    };
  }, [callback, delay]);
}

/**
 * Schedule a one-time callback that respects component unmounting.
 * Returns a function to cancel the timeout early.
 */
export function useScheduleCallback(callback: () => void, delay: number): () => void {
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isUnmountedRef = useRef(false);

  useEffect(() => {
    return () => {
      isUnmountedRef.current = true;
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
        timeoutRef.current = null;
      }
    };
  }, []);

  const schedule = () => {
    // Clear any existing timeout
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }

    isUnmountedRef.current = false;
    timeoutRef.current = setTimeout(() => {
      if (!isUnmountedRef.current) {
        callback();
      }
      timeoutRef.current = null;
    }, delay);

    // Return cancel function
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
        timeoutRef.current = null;
      }
    };
  };

  return schedule;
}
