// file: web/src/services/eventSourceManager.ts
// version: 1.1.0
// guid: 5a9b8c7d-6e5f-4a3b-2c1d-0e9f8a7b6c5d

export type EventSourceStatus = {
  state: 'open' | 'reconnecting' | 'closed' | 'error';
  attempt: number;
  delayMs?: number;
  error?: Event;
};

export type EventSourceEvent = {
  type: string;
  id?: string;
  timestamp?: string;
  data?: Record<string, unknown>;
  [key: string]: unknown;
};

export type EventSourceListener = (event: EventSourceEvent) => void;
export type EventSourceStatusListener = (status: EventSourceStatus) => void;

type Unsubscribe = () => void;

const baseDelayMs = 3000;
const maxDelayMs = 60000;

const createDelay = (attempt: number) =>
  Math.min(baseDelayMs * Math.pow(2, Math.max(attempt - 1, 0)), maxDelayMs);

export const createEventSourceManager = (url = '/api/events') => {
  let eventSource: EventSource | null = null;
  let reconnectAttempt = 0;
  let reconnectTimer: number | null = null;
  let connectTimeoutTimer: number | null = null;
  let connecting = false;

  const listeners = new Set<EventSourceListener>();
  const statusListeners = new Set<EventSourceStatusListener>();

  // Max reconnect attempts before giving up (avoids infinite connection churn)
  const maxReconnectAttempts = 5;
  // Timeout for initial connection — abort if server doesn't respond
  const connectTimeoutMs = 8000;

  const notifyStatus = (status: EventSourceStatus) => {
    statusListeners.forEach((listener) => listener(status));
  };

  const hasSubscribers = () => listeners.size > 0 || statusListeners.size > 0;

  const clearConnectTimeout = () => {
    if (connectTimeoutTimer) {
      window.clearTimeout(connectTimeoutTimer);
      connectTimeoutTimer = null;
    }
  };

  const close = () => {
    clearConnectTimeout();
    if (reconnectTimer) {
      window.clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    connecting = false;
    reconnectAttempt = 0;
    notifyStatus({ state: 'closed', attempt: 0 });
  };

  const handleMessage = (ev: MessageEvent) => {
    if (!ev.data) return;
    try {
      const parsed = JSON.parse(ev.data) as EventSourceEvent;
      if (!parsed || typeof parsed.type !== 'string') return;
      listeners.forEach((listener) => listener(parsed));
    } catch {
      // Ignore malformed events.
    }
  };

  const scheduleReconnect = (error?: Event) => {
    if (!hasSubscribers()) {
      close();
      return;
    }
    if (reconnectTimer) return;

    reconnectAttempt += 1;

    // Stop reconnecting after too many failures to avoid blocking browser connections
    if (reconnectAttempt > maxReconnectAttempts) {
      notifyStatus({ state: 'closed', attempt: reconnectAttempt });
      return;
    }

    const delayMs = createDelay(reconnectAttempt);
    notifyStatus({
      state: 'reconnecting',
      attempt: reconnectAttempt,
      delayMs,
      error,
    });

    reconnectTimer = window.setTimeout(() => {
      reconnectTimer = null;
      connect();
    }, delayMs);
  };

  const connect = () => {
    if (eventSource || connecting || !hasSubscribers()) return;

    connecting = true;
    eventSource = new EventSource(url);

    // Abort the connection if the server doesn't respond in time
    clearConnectTimeout();
    connectTimeoutTimer = window.setTimeout(() => {
      connectTimeoutTimer = null;
      if (connecting && eventSource) {
        eventSource.close();
        eventSource = null;
        connecting = false;
        notifyStatus({ state: 'error', attempt: reconnectAttempt });
        scheduleReconnect();
      }
    }, connectTimeoutMs);

    eventSource.onmessage = handleMessage;
    eventSource.onerror = (error) => {
      clearConnectTimeout();
      connecting = false;
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
      notifyStatus({ state: 'error', attempt: reconnectAttempt, error });
      scheduleReconnect(error);
    };
    eventSource.onopen = () => {
      clearConnectTimeout();
      connecting = false;
      reconnectAttempt = 0;
      notifyStatus({ state: 'open', attempt: 0 });
    };
  };

  const subscribe = (
    listener: EventSourceListener,
    statusListener?: EventSourceStatusListener
  ): Unsubscribe => {
    listeners.add(listener);
    if (statusListener) {
      statusListeners.add(statusListener);
    }
    // Reset attempts when a new subscriber arrives (e.g. page navigation)
    if (reconnectAttempt > maxReconnectAttempts) {
      reconnectAttempt = 0;
    }
    connect();

    return () => {
      listeners.delete(listener);
      if (statusListener) {
        statusListeners.delete(statusListener);
      }
      if (!hasSubscribers()) {
        close();
      }
    };
  };

  return {
    subscribe,
    close,
  };
};

export const eventSourceManager = createEventSourceManager();
