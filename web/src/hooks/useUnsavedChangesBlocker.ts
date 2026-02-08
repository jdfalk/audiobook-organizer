// file: web/src/hooks/useUnsavedChangesBlocker.ts
// version: 1.0.0
// guid: e3f4a5b6-c7d8-9e0f-1a2b-3c4d5e6f7a8b

import { useEffect, useCallback, useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';

interface Blocker {
  state: 'idle' | 'blocked';
  proceed: (() => void) | null;
  reset: (() => void) | null;
}

/**
 * Custom hook that replaces react-router's useBlocker without requiring a data router.
 * Warns before browser unload and intercepts in-app navigation when there are unsaved changes.
 */
export function useUnsavedChangesBlocker(shouldBlock: boolean): Blocker {
  const [blocked, setBlocked] = useState(false);
  const [pendingPath, setPendingPath] = useState<string | null>(null);
  const navigate = useNavigate();
  const location = useLocation();

  // Handle browser beforeunload
  useEffect(() => {
    if (!shouldBlock) return;
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [shouldBlock]);

  // Override navigate to intercept in-app navigation
  // We do this by patching pushState/replaceState
  useEffect(() => {
    if (!shouldBlock) {
      setBlocked(false);
      setPendingPath(null);
      return;
    }

    const originalPushState = history.pushState.bind(history);
    const originalReplaceState = history.replaceState.bind(history);

    const intercept = (
      original: typeof history.pushState,
      data: unknown,
      unused: string,
      url?: string | URL | null
    ) => {
      if (url && typeof url === 'string' && url !== location.pathname) {
        setPendingPath(url);
        setBlocked(true);
        return; // Block the navigation
      }
      original(data, unused, url);
    };

    history.pushState = function (data, unused, url) {
      intercept(originalPushState, data, unused, url);
    };
    history.replaceState = function (data, unused, url) {
      intercept(originalReplaceState, data, unused, url);
    };

    return () => {
      history.pushState = originalPushState;
      history.replaceState = originalReplaceState;
    };
  }, [shouldBlock, location.pathname]);

  const proceed = useCallback(() => {
    setBlocked(false);
    if (pendingPath) {
      const path = pendingPath;
      setPendingPath(null);
      // Use setTimeout to ensure state is cleaned up before navigating
      setTimeout(() => navigate(path), 0);
    }
  }, [pendingPath, navigate]);

  const reset = useCallback(() => {
    setBlocked(false);
    setPendingPath(null);
  }, []);

  return {
    state: blocked ? 'blocked' : 'idle',
    proceed: blocked ? proceed : null,
    reset: blocked ? reset : null,
  };
}
