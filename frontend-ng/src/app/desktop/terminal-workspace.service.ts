// SPDX-License-Identifier: EUPL-1.2

import { Injectable, OnDestroy, inject, signal } from '@angular/core';
import { ConnectionManagerService } from '../connection-manager.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';
import {
  TerminalWorkspace,
  TerminalWorkspaceSnapshot,
  parseTerminalWorkspace,
  parseTerminalWorkspaceSnapshot,
} from './terminal-workspace.models';

const DESKTOP_STATE_SERVICE = 'dappco.re/lthn/desktop/pkg/desktopstate.WailsService';
const SAVE_DELAY_MS = 250;

@Injectable({ providedIn: 'root' })
export class TerminalWorkspaceService implements OnDestroy {
  private readonly surface = inject(SurfaceBridgeService);
  private readonly connection = inject(ConnectionManagerService);
  private revision = 0;
  private demoSnapshot: TerminalWorkspaceSnapshot = emptySnapshot();
  private pendingWorkspace: TerminalWorkspace | null = null;
  private saveTimer: ReturnType<typeof setTimeout> | null = null;
  private saveChain: Promise<void> = Promise.resolve();

  readonly error = signal('');

  isOffline(): boolean {
    return this.connection.offline();
  }

  async load(): Promise<TerminalWorkspaceSnapshot> {
    try {
      const snapshot = this.isOffline()
        ? copySnapshot(this.demoSnapshot)
        : parseTerminalWorkspaceSnapshot(
            await this.surface.call(`${DESKTOP_STATE_SERVICE}.LoadTerminalWorkspace`),
          );
      this.revision = snapshot.revision;
      this.error.set('');
      return snapshot;
    } catch (error) {
      this.error.set(messageFor(error));
      throw error;
    }
  }

  schedule(workspace: TerminalWorkspace): void {
    this.pendingWorkspace = parseTerminalWorkspace(workspace);
    if (this.saveTimer !== null) clearTimeout(this.saveTimer);
    this.saveTimer = setTimeout(() => {
      this.saveTimer = null;
      this.enqueuePendingSave();
    }, SAVE_DELAY_MS);
  }

  async flush(): Promise<void> {
    if (this.saveTimer !== null) {
      clearTimeout(this.saveTimer);
      this.saveTimer = null;
      this.enqueuePendingSave();
    }
    await this.saveChain;
  }

  ngOnDestroy(): void {
    void this.flush();
  }

  private enqueuePendingSave(): void {
    const workspace = this.pendingWorkspace;
    if (!workspace) return;
    this.pendingWorkspace = null;
    this.saveChain = this.saveChain
      .catch(() => undefined)
      .then(() => this.save(workspace))
      .catch((error: unknown) => {
        this.error.set(messageFor(error));
      });
  }

  private async save(workspace: TerminalWorkspace): Promise<void> {
    if (this.isOffline()) {
      this.demoSnapshot = {
        version: 1,
        revision: this.demoSnapshot.revision + 1,
        updatedAt: new Date().toISOString(),
        workspace,
      };
      this.revision = this.demoSnapshot.revision;
      this.error.set('');
      return;
    }

    const snapshot = parseTerminalWorkspaceSnapshot(
      await this.surface.call(`${DESKTOP_STATE_SERVICE}.SaveTerminalWorkspace`, [
        {
          expectedRevision: this.revision,
          workspace,
        },
      ]),
    );
    this.revision = snapshot.revision;
    this.error.set('');
  }
}

function emptySnapshot(): TerminalWorkspaceSnapshot {
  return {
    version: 1,
    revision: 0,
    updatedAt: '',
    workspace: {
      activeKey: '',
      tabs: [],
    },
  };
}

function copySnapshot(snapshot: TerminalWorkspaceSnapshot): TerminalWorkspaceSnapshot {
  return {
    ...snapshot,
    workspace: {
      activeKey: snapshot.workspace.activeKey,
      tabs: snapshot.workspace.tabs.map((tab) => ({
        ...tab,
        workspace: { ...tab.workspace },
      })),
    },
  };
}

function messageFor(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
