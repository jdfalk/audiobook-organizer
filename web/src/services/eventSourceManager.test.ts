// file: web/src/services/eventSourceManager.test.ts
// version: 1.0.0
// guid: c1d2e3f4-a5b6-7c8d-9e0f-1a2b3c4d5e6f

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { createEventSourceManager } from './eventSourceManager';

type EventSourceHandler = ((event: MessageEvent) => void) | null;

type ErrorHandler = ((event: Event) => void) | null;

type OpenHandler = (() => void) | null;

class TestEventSource {
  static instances: TestEventSource[] = [];
  onmessage: EventSourceHandler = null;
  onerror: ErrorHandler = null;
  onopen: OpenHandler = null;
  readyState = 1;
  url: string;

  constructor(url: string) {
    this.url = url;
    TestEventSource.instances.push(this);
  }

  close() {
    this.readyState = 2;
  }

  emitOpen() {
    this.onopen?.();
  }

  emitMessage(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent);
  }

  emitError() {
    this.onerror?.(new Event('error'));
  }
}

describe('eventSourceManager', () => {
  beforeEach(() => {
    TestEventSource.instances = [];
    vi.stubGlobal('EventSource', TestEventSource);
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('delivers parsed events to subscribers', () => {
    const manager = createEventSourceManager('/api/events');
    const received: string[] = [];

    const unsubscribe = manager.subscribe((event) => {
      received.push(event.type);
    });

    expect(TestEventSource.instances).toHaveLength(1);
    TestEventSource.instances[0].emitMessage({ type: 'operation.log' });

    expect(received).toEqual(['operation.log']);
    unsubscribe();
  });

  it('reconnects with exponential backoff', () => {
    const manager = createEventSourceManager('/api/events');
    const statuses: Array<{
      state: string;
      attempt: number;
      delayMs?: number;
    }> = [];

    manager.subscribe(
      () => {},
      (status) => {
        statuses.push({
          state: status.state,
          attempt: status.attempt,
          delayMs: status.delayMs,
        });
      }
    );

    expect(TestEventSource.instances).toHaveLength(1);
    TestEventSource.instances[0].emitError();

    const reconnectStatus = statuses.find(
      (status) => status.state === 'reconnecting'
    );
    expect(reconnectStatus?.attempt).toBe(1);
    expect(reconnectStatus?.delayMs).toBe(3000);

    vi.advanceTimersByTime(3000);

    expect(TestEventSource.instances).toHaveLength(2);
  });
});
