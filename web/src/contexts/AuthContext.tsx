// file: web/src/contexts/AuthContext.tsx
// version: 1.0.0
// guid: 2b3c4d5e-6f70-4819-a2b3-c4d5e6f70819

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import * as api from '../services/api';
import { ApiError, type AuthUser } from '../services/api';

interface AuthContextValue {
  initialized: boolean;
  loading: boolean;
  user: AuthUser | null;
  requiresAuth: boolean;
  bootstrapReady: boolean;
  isAuthenticated: boolean;
  refresh: () => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  setupAdmin: (payload: {
    username: string;
    password: string;
    email?: string;
  }) => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [initialized, setInitialized] = useState(false);
  const [loading, setLoading] = useState(false);
  const [user, setUser] = useState<AuthUser | null>(null);
  const [requiresAuth, setRequiresAuth] = useState(false);
  const [bootstrapReady, setBootstrapReady] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const status = await api.getAuthStatus();
      setRequiresAuth(status.requires_auth);
      setBootstrapReady(status.bootstrap_ready);

      if (!status.requires_auth) {
        setUser(null);
        return;
      }

      const me = await api.getMe();
      setUser(me);
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        setUser(null);
      } else {
        // Treat auth endpoint failures as unauthenticated until next retry.
        setUser(null);
      }
    } finally {
      setInitialized(true);
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const login = useCallback(async (username: string, password: string) => {
    const response = await api.login({ username, password });
    setUser(response.user);
    setRequiresAuth(true);
    setBootstrapReady(false);
  }, []);

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } finally {
      setUser(null);
    }
  }, []);

  const setupAdmin = useCallback(
    async (payload: { username: string; password: string; email?: string }) => {
      await api.setupAdmin(payload);
      setBootstrapReady(false);
      setRequiresAuth(true);
    },
    []
  );

  const value = useMemo<AuthContextValue>(
    () => ({
      initialized,
      loading,
      user,
      requiresAuth,
      bootstrapReady,
      isAuthenticated: Boolean(user),
      refresh,
      login,
      logout,
      setupAdmin,
    }),
    [
      initialized,
      loading,
      user,
      requiresAuth,
      bootstrapReady,
      refresh,
      login,
      logout,
      setupAdmin,
    ]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuth must be used within AuthProvider');
  }
  return ctx;
}
