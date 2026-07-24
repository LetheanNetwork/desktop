import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Directive,
  Input,
  OnChanges,
  OnDestroy,
  OnInit,
  SimpleChanges,
  computed,
  inject,
  signal,
} from '@angular/core';
import { Win } from '../desktop.data';
import { SurfaceBridgeService } from './surface-bridge.service';

import '../../../kit/lthn-charts';
import '../../../kit/lthn-core';

export type SurfaceTone = 'brand' | 'ok' | 'warn' | 'danger' | 'muted';
export type SurfaceKind = 'list' | 'board' | 'dashboard' | 'editor' | 'terminal' | 'calendar';

export interface SurfaceMetric {
  readonly label: string;
  readonly value: string;
  readonly hint?: string;
  readonly tone?: SurfaceTone;
}

export interface SurfaceRow {
  readonly id: string;
  readonly title: string;
  readonly meta?: string;
  readonly detail?: string;
  readonly status?: string;
  readonly value?: string;
  readonly secondary?: string;
  readonly tags?: readonly string[];
  readonly progress?: number;
  readonly tone?: SurfaceTone;
}

export interface SurfaceColumn {
  readonly id: string;
  readonly title: string;
  readonly value?: string;
  readonly cards: readonly SurfaceRow[];
}

export interface SurfaceSection {
  readonly title: string;
  readonly body?: string;
  readonly items?: readonly string[];
  readonly tone?: SurfaceTone;
}

export interface SurfaceFilter {
  readonly id: string;
  readonly label: string;
}

export interface SurfaceAction {
  readonly id: string;
  readonly label: string;
  readonly icon: string;
  readonly kind?: 'refresh' | 'add' | 'run' | 'clear' | 'copy';
}

export interface SurfaceConfig {
  readonly id: string;
  readonly title: string;
  readonly subtitle: string;
  readonly icon: string;
  readonly eyebrow?: string;
  readonly kind?: SurfaceKind;
  readonly searchPlaceholder?: string;
  readonly emptyLabel?: string;
  readonly filters?: readonly SurfaceFilter[];
  readonly actions?: readonly SurfaceAction[];
  readonly metrics?: readonly SurfaceMetric[];
  readonly rows?: readonly SurfaceRow[];
  readonly columns?: readonly SurfaceColumn[];
  readonly sections?: readonly SurfaceSection[];
  readonly footer: string;
  readonly chart?: readonly number[];
  readonly editorLabel?: string;
  readonly editorText?: string;
  readonly endpoint?: string;
  readonly endpointPayload?: 'query' | 'ask';
  readonly loadEndpoint?: string;
  readonly bridgeMethod?: string;
  readonly bridgeArgs?: readonly unknown[];
  readonly bridgeInput?: 'json' | 'text';
  readonly liveKeys?: readonly string[];
  readonly moveBridgeMethod?: string;
  readonly pollMs?: number;
  readonly conflictReloadService?: string;
}

/**
 * Input contract shared by every routed surface. NgRx owns `win`; each
 * surface keeps its local interaction state in signals.
 */
@Directive({
  host: {
    style: 'display:block;width:100%;height:100%;min-height:0',
  },
})
export abstract class SurfaceRoute {
  @Input({ required: true }) win!: Win;
}

@Component({
  selector: 'lthn-surface-page',
  imports: [],
  templateUrl: './surface-page.html',
  styleUrl: './surface-page.scss',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SurfacePage implements OnChanges, OnInit, OnDestroy {
  private readonly bridge = inject(SurfaceBridgeService);
  private loadVersion = 0;
  private liveLoading = false;
  private pollHandle: number | undefined;
  private drag: { card: SurfaceRow; source: string } | null = null;

  @Input({ required: true }) config!: SurfaceConfig;

  readonly query = signal('');
  readonly activeFilter = signal('all');
  readonly selectedId = signal('');
  readonly rows = signal<readonly SurfaceRow[]>([]);
  readonly columns = signal<readonly SurfaceColumn[]>([]);
  readonly editor = signal('');
  readonly result = signal('');
  readonly notice = signal('');
  readonly busy = signal(false);

  private readonly conflictReload = (event: Event): void => {
    const service = (event as CustomEvent<{ service?: unknown }>).detail?.service;
    if (typeof service === 'string' && service === this.config.conflictReloadService) {
      void this.loadLive(false);
    }
  };

  readonly visibleRows = computed(() => {
    const query = this.query().trim().toLocaleLowerCase('en-GB');
    const filter = this.activeFilter();
    return this.rows().filter((row) => {
      const matchesFilter =
        filter === 'all' ||
        row.status === filter ||
        row.tone === filter ||
        row.tags?.includes(filter);
      if (!matchesFilter) return false;
      if (!query) return true;
      return [
        row.id,
        row.title,
        row.meta,
        row.detail,
        row.status,
        row.value,
        row.secondary,
        ...(row.tags ?? []),
      ]
        .filter(Boolean)
        .join(' ')
        .toLocaleLowerCase('en-GB')
        .includes(query);
    });
  });

  readonly visibleColumns = computed(() => {
    const query = this.query().trim().toLocaleLowerCase('en-GB');
    if (!query) return this.columns();
    return this.columns().map((column) => ({
      ...column,
      cards: column.cards.filter((card) =>
        [card.id, card.title, card.meta, card.detail, ...(card.tags ?? [])]
          .filter(Boolean)
          .join(' ')
          .toLocaleLowerCase('en-GB')
          .includes(query),
      ),
    }));
  });

  ngOnChanges(changes: SimpleChanges): void {
    if (!changes['config'] || !this.config) return;
    this.rows.set(this.config.rows?.map((row) => ({ ...row })) ?? []);
    this.columns.set(
      this.config.columns?.map((column) => ({
        ...column,
        cards: column.cards.map((card) => ({ ...card })),
      })) ?? [],
    );
    this.editor.set(this.config.editorText ?? '');
    this.query.set('');
    this.activeFilter.set('all');
    this.selectedId.set('');
    this.result.set('');
    this.notice.set('');
    const version = ++this.loadVersion;
    if ((this.config.bridgeMethod || this.config.loadEndpoint) && !this.config.bridgeInput) {
      queueMicrotask(() => {
        if (version === this.loadVersion) void this.loadLive(false);
      });
    }
  }

  ngOnInit(): void {
    window.addEventListener('lthn:conflict:reload-requested', this.conflictReload);
    if (
      this.config.pollMs &&
      this.config.pollMs > 0 &&
      (this.config.bridgeMethod || this.config.loadEndpoint) &&
      !this.config.bridgeInput
    ) {
      this.pollHandle = window.setInterval(() => {
        void this.loadLive(false);
      }, this.config.pollMs);
    }
  }

  ngOnDestroy(): void {
    this.loadVersion++;
    window.removeEventListener('lthn:conflict:reload-requested', this.conflictReload);
    if (this.pollHandle !== undefined) {
      window.clearInterval(this.pollHandle);
      this.pollHandle = undefined;
    }
  }

  setQuery(event: Event): void {
    this.query.set((event.target as HTMLInputElement).value);
  }

  setEditor(event: Event): void {
    this.editor.set((event.target as HTMLTextAreaElement).value);
  }

  selectFilter(id: string): void {
    this.activeFilter.set(this.activeFilter() === id ? 'all' : id);
  }

  selectRow(id: string): void {
    this.selectedId.set(this.selectedId() === id ? '' : id);
  }

  async runAction(action: SurfaceAction): Promise<void> {
    if (this.busy()) return;
    this.notice.set('');

    if (action.kind === 'clear') {
      this.editor.set('');
      this.result.set('');
      return;
    }
    if (action.kind === 'copy') {
      try {
        await navigator.clipboard.writeText(this.editor());
        this.notice.set(
          $localize`:Surface copy success@@surface.common.copied:Copied to clipboard.`,
        );
      } catch {
        this.notice.set(
          $localize`:Surface copy failure@@surface.common.copyFailed:Clipboard access is unavailable.`,
        );
      }
      return;
    }
    if (action.kind === 'add') {
      const id = `local-${Date.now()}`;
      if (this.config.kind === 'board' && this.columns().length) {
        this.columns.update((columns) =>
          columns.map((column, index) =>
            index === 0
              ? {
                  ...column,
                  cards: [
                    {
                      id,
                      title: $localize`:New surface item@@surface.common.newItem:New item`,
                      meta: $localize`:New surface item state@@surface.common.localDraft:Local draft`,
                      status: 'draft',
                      tone: 'muted',
                    },
                    ...column.cards,
                  ],
                }
              : column,
          ),
        );
        this.selectedId.set(id);
        return;
      }
      this.rows.update((rows) => [
        {
          id,
          title: $localize`:New surface item@@surface.common.newItem:New item`,
          meta: $localize`:New surface item state@@surface.common.localDraft:Local draft`,
          status: 'draft',
          tone: 'muted',
        },
        ...rows,
      ]);
      this.selectedId.set(id);
      return;
    }

    this.busy.set(true);
    try {
      if (action.kind === 'run' && this.config.endpoint) {
        const value = await this.bridge.request(this.config.endpoint, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(
            this.config.endpointPayload === 'ask'
              ? { prompt: this.editor(), active_tab: 'models' }
              : { query: this.editor(), body: this.editor(), sql: this.editor() },
          ),
        });
        this.result.set(JSON.stringify(value, null, 2));
        this.notice.set(
          $localize`:Surface run success@@surface.common.queryComplete:Query complete.`,
        );
        return;
      }
      if (this.config.bridgeMethod || this.config.loadEndpoint) {
        await this.loadLive(true);
        return;
      }
      this.notice.set(
        $localize`:Surface local state@@surface.common.localData:Showing the local, offline-safe data set.`,
      );
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      this.notice.set(
        $localize`:Surface backend failure@@surface.common.backendUnavailable:Backend unavailable; the existing local data has been kept.` +
          (message ? ` ${message}` : ''),
      );
    } finally {
      this.busy.set(false);
    }
  }

  dragStart(event: DragEvent, card: SurfaceRow, source: string): void {
    this.drag = { card, source };
    event.dataTransfer?.setData(
      'application/x-lthn-surface-card',
      JSON.stringify({ id: card.id, source }),
    );
    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move';
  }

  allowDrop(event: DragEvent): void {
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
  }

  drop(event: DragEvent, target: string): void {
    event.preventDefault();
    const drag = this.drag;
    this.drag = null;
    if (!drag || drag.source === target) return;

    const previous = this.columns();
    this.columns.update((columns) =>
      columns.map((column) => {
        if (column.id === drag.source) {
          return { ...column, cards: column.cards.filter(({ id }) => id !== drag.card.id) };
        }
        if (column.id === target) {
          return { ...column, cards: [...column.cards, drag.card] };
        }
        return column;
      }),
    );
    if (this.config.moveBridgeMethod) {
      void this.bridge
        .call(this.config.moveBridgeMethod, [{ dealId: drag.card.id, toStage: target }])
        .then((value) => {
          this.applyLiveValue(value);
          this.emitMoved(drag.card, drag.source, target);
        })
        .catch((error: unknown) => {
          this.columns.set(previous);
          const message = error instanceof Error ? error.message : String(error);
          this.notice.set(
            $localize`:Board move failure@@surface.common.moveFailed:The move was rejected and has been reverted.` +
              (message ? ` ${message}` : ''),
          );
        });
      return;
    }
    this.emitMoved(drag.card, drag.source, target);
  }

  private emitMoved(card: SurfaceRow, source: string, target: string): void {
    window.dispatchEvent(
      new CustomEvent('lthn:surface:moved', {
        detail: { item_id: card.id, from: source, to: target },
      }),
    );
    if (this.config.id === 'sales-pipeline') {
      window.dispatchEvent(
        new CustomEvent('lthn:sales:moved', {
          detail: {
            deal_id: card.id,
            customer: card.title,
            from_stage: source,
            to_stage: target,
          },
        }),
      );
      if (target === 'won' || target === 'lost') {
        this.notice.set(
          $localize`:Pipeline terminal move@@surface.sales.pipeline.terminalMove:${card.title}:customer: moved to ${target}:stage:. Confirm the terminal transition in the deal record.`,
        );
      }
    }
  }

  progress(value: number | undefined): string {
    const bounded = Math.max(0, Math.min(100, value ?? 0));
    return `${bounded}%`;
  }

  private applyLiveValue(value: unknown): void {
    if (!value || (typeof value !== 'object' && !Array.isArray(value))) return;
    const record = Array.isArray(value) ? { rows: value } : (value as Record<string, unknown>);
    if (this.config.kind === 'board' && this.applyLiveColumns(record['columns'])) return;

    const candidates = this.config.liveKeys ?? [
      'rows',
      'items',
      'events',
      'issues',
      'tasks',
      'contacts',
      'campaigns',
      'posts',
      'models',
      'runs',
      'repos',
      'envs',
      'history',
      'segments',
      'sources',
      'pages',
      'documents',
      'recents',
      'threads',
      'incidents',
      'books',
      'sites',
      'quarters',
      'deals',
      'workspaces',
    ];
    const live = candidates
      .map((key) => record[key])
      .find((candidate): candidate is unknown[] => Array.isArray(candidate));
    if (!live?.length) return;
    if (this.config.kind === 'board' && this.applyLiveBoardRows(live)) return;

    const rows = live.slice(0, 2_000).flatMap((candidate, index): SurfaceRow[] => {
      if (!candidate || typeof candidate !== 'object') return [];
      const row = candidate as Record<string, unknown>;
      const title = [
        row['title'],
        row['name'],
        row['subject'],
        row['customer'],
        row['repo'],
        row['environment'],
        row['service'],
        row['quarter'],
        row['path'],
        row['summary'],
        row['Summary'],
        row['event'],
        row['filename'],
        row['id'],
      ].find((field) => typeof field === 'string' && field.length > 0);
      if (typeof title !== 'string') return [];
      return [
        {
          id: String(row['id'] ?? row['ID'] ?? index),
          title,
          meta: String(
            row['role'] ??
              row['Project'] ??
              row['project'] ??
              row['channel'] ??
              row['owner'] ??
              row['assignee'] ??
              row['base_model'] ??
              '',
          ),
          detail: String(
            row['body'] ?? row['description'] ?? row['detail'] ?? row['message'] ?? '',
          ),
          status: String(
            row['status'] ?? row['state'] ?? row['State'] ?? row['outcome'] ?? row['build'] ?? '',
          ),
          value: String(
            row['value'] ?? row['size'] ?? row['amount'] ?? row['total'] ?? row['count'] ?? '',
          ),
        },
      ];
    });
    if (rows.length) this.rows.set(rows);
  }

  private async loadLive(announce: boolean): Promise<void> {
    if (this.liveLoading) return;
    this.liveLoading = true;
    try {
      const value = this.config.bridgeMethod
        ? await this.bridge.call(
            this.config.bridgeMethod,
            this.config.bridgeInput ? [this.bridgeInputValue()] : (this.config.bridgeArgs ?? []),
          )
        : await this.bridge.request(this.config.loadEndpoint!, { method: 'GET' });
      this.applyLiveValue(value);
      if (announce) {
        this.notice.set(
          $localize`:Surface refresh success@@surface.common.liveDataLoaded:Live data loaded.`,
        );
      }
    } catch (error) {
      if (!announce) return;
      const message = error instanceof Error ? error.message : String(error);
      this.notice.set(
        $localize`:Surface backend failure@@surface.common.backendUnavailable:Backend unavailable; the existing local data has been kept.` +
          (message ? ` ${message}` : ''),
      );
    } finally {
      this.liveLoading = false;
    }
  }

  private bridgeInputValue(): unknown {
    if (this.config.bridgeInput === 'text') return this.editor();
    if (this.config.bridgeInput === 'json') {
      const value = JSON.parse(this.editor()) as unknown;
      if (!value || typeof value !== 'object' || Array.isArray(value)) {
        throw new Error('The request must be a JSON object.');
      }
      return value;
    }
    return undefined;
  }

  private applyLiveColumns(value: unknown): boolean {
    if (!Array.isArray(value) || !value.length) return false;
    const columns = value.slice(0, 100).flatMap((candidate, columnIndex): SurfaceColumn[] => {
      if (!candidate || typeof candidate !== 'object') return [];
      const column = candidate as Record<string, unknown>;
      const cardsValue = ['cards', 'items', 'deals']
        .map((key) => column[key])
        .find((cards): cards is unknown[] => Array.isArray(cards));
      if (!cardsValue) return [];
      const id = String(column['id'] ?? column['stage'] ?? columnIndex);
      const title = String(column['title'] ?? column['name'] ?? column['label'] ?? id);
      const cards = cardsValue.slice(0, 2_000).flatMap((item, cardIndex): SurfaceRow[] => {
        if (!item || typeof item !== 'object') return [];
        const card = item as Record<string, unknown>;
        const cardTitle = [
          card['title'],
          card['name'],
          card['customer'],
          card['subject'],
          card['c'],
          card['t'],
        ].find((field) => typeof field === 'string' && field.length > 0);
        if (typeof cardTitle !== 'string') return [];
        return [
          {
            id: String(card['id'] ?? cardIndex),
            title: cardTitle,
            detail: String(card['detail'] ?? card['description'] ?? card['t'] ?? ''),
            meta: String(card['meta'] ?? card['owner'] ?? ''),
            value: String(card['value'] ?? card['amount'] ?? card['v'] ?? ''),
            status: String(card['status'] ?? card['state'] ?? id),
          },
        ];
      });
      return [{ id, title, value: String(column['value'] ?? ''), cards }];
    });
    if (!columns.length) return false;
    this.columns.set(columns);
    return true;
  }

  private applyLiveBoardRows(value: readonly unknown[]): boolean {
    const existing = this.columns();
    if (!existing.length) return false;
    const byColumn = new Map(existing.map(({ id }) => [id, [] as SurfaceRow[]]));
    const aliases: Record<string, string> = {
      open: 'todo',
      todo: 'todo',
      backlog: 'todo',
      in_progress: 'doing',
      active: 'doing',
      doing: 'doing',
      review: 'review',
      completed: 'done',
      closed: 'done',
      done: 'done',
    };
    for (const [index, candidate] of value.slice(0, 2_000).entries()) {
      if (!candidate || typeof candidate !== 'object') continue;
      const row = candidate as Record<string, unknown>;
      const state = String(row['state'] ?? row['status'] ?? '');
      const target = byColumn.has(state) ? state : aliases[state];
      const cards = target ? byColumn.get(target) : undefined;
      const title = row['title'] ?? row['name'] ?? row['summary'];
      if (!cards || typeof title !== 'string') continue;
      cards.push({
        id: String(row['id'] ?? index),
        title,
        detail: String(row['description'] ?? row['body'] ?? ''),
        meta: String(row['assignee'] ?? row['owner'] ?? row['project'] ?? ''),
        status: state,
        value: String(row['priority'] ?? row['value'] ?? ''),
      });
    }
    if (![...byColumn.values()].some((cards) => cards.length)) return false;
    this.columns.set(
      existing.map((column) => ({ ...column, cards: byColumn.get(column.id) ?? [] })),
    );
    return true;
  }
}
