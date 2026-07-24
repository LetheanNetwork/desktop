import {
  AfterViewInit,
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  EventEmitter,
  Input,
  OnChanges,
  OnDestroy,
  Output,
  SimpleChanges,
  ViewChild,
  inject,
  signal,
} from '@angular/core';
import { ClipboardAddon } from '@xterm/addon-clipboard';
import { FitAddon } from '@xterm/addon-fit';
import { SearchAddon } from '@xterm/addon-search';
import { UnicodeGraphemesAddon } from '@xterm/addon-unicode-graphemes';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import { SurfaceBridgeService } from '../surface-bridge.service';

const TERMINAL_SERVICE = 'dappco.re/lthn/desktop/pkg/terminal.Service';

export interface TerminalTab {
  readonly key: string;
  readonly title: string;
  readonly repo?: string;
  readonly cwd?: string;
  readonly attachId?: string;
  readonly command?: readonly string[];
  readonly shared?: boolean;
  readonly exited?: boolean;
}

export interface TerminalReady {
  readonly key: string;
  readonly title: string;
  readonly cwd: string;
  readonly shell: string;
}

@Component({
  selector: 'lthn-agent-terminal-session',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div #host class="terminal-host" [attr.aria-label]="tab.title"></div>
    @if (searchOpen()) {
      <div class="search">
        <i class="fa-solid fa-magnifying-glass" aria-hidden="true"></i>
        <input
          #searchInput
          type="search"
          placeholder="Find in terminal"
          i18n-placeholder="Terminal search placeholder@@surface.agents.terminal.search"
          [value]="searchTerm()"
          (input)="search($event)"
          (keydown)="searchKey($event)"
        />
        <span>{{ searchCount() }}</span>
        <button
          type="button"
          (click)="findPrevious()"
          aria-label="Previous match"
          i18n-aria-label="Terminal previous match@@surface.agents.terminal.previousMatch"
        >
          <i class="fa-solid fa-chevron-up" aria-hidden="true"></i>
        </button>
        <button
          type="button"
          (click)="findNext()"
          aria-label="Next match"
          i18n-aria-label="Terminal next match@@surface.agents.terminal.nextMatch"
        >
          <i class="fa-solid fa-chevron-down" aria-hidden="true"></i>
        </button>
        <button
          type="button"
          (click)="closeSearch()"
          aria-label="Close search"
          i18n-aria-label="Terminal close search@@surface.agents.terminal.closeSearch"
        >
          <i class="fa-solid fa-xmark" aria-hidden="true"></i>
        </button>
      </div>
    }
    @if (error()) {
      <div class="error" role="alert">{{ error() }}</div>
    }
  `,
  styles: `
    :host {
      position: relative;
      display: flex;
      flex: 1;
      min-width: 0;
      min-height: 0;
      overflow: hidden;
      background: #0b0e11;
    }
    .terminal-host {
      flex: 1;
      min-width: 0;
      min-height: 0;
      padding: 8px 8px 4px;
    }
    .terminal-host ::ng-deep .xterm {
      height: 100%;
    }
    .search {
      position: absolute;
      z-index: 2;
      top: 8px;
      right: 20px;
      display: flex;
      align-items: center;
      gap: 5px;
      padding: 5px 6px;
      color: var(--fg-3);
      background: var(--ink-2);
      border: 1px solid var(--line-2);
      border-radius: 7px;
      box-shadow: 0 8px 24px rgba(0, 0, 0, 0.28);
    }
    .search input {
      width: 190px;
      color: var(--fg-0);
      background: transparent;
      border: 0;
      outline: 0;
      font: 10px var(--font-mono);
    }
    .search span {
      min-width: 30px;
      color: var(--fg-3);
      font: 8px var(--font-mono);
      text-align: center;
    }
    .search button {
      display: grid;
      width: 23px;
      height: 23px;
      place-items: center;
      padding: 0;
      color: var(--fg-2);
      background: transparent;
      border: 0;
      border-radius: 4px;
      cursor: pointer;
    }
    .search button:hover {
      color: var(--fg-0);
      background: rgba(255, 255, 255, 0.06);
    }
    .error {
      position: absolute;
      inset: auto 12px 12px;
      padding: 8px 10px;
      color: var(--danger-300);
      background: rgba(239, 68, 68, 0.09);
      border: 1px solid rgba(239, 68, 68, 0.22);
      border-radius: 6px;
      font: 9px var(--font-mono);
    }
  `,
})
export class AgentTerminalSession implements AfterViewInit, OnChanges, OnDestroy {
  private readonly bridge = inject(SurfaceBridgeService);
  private terminal?: Terminal;
  private fitAddon?: FitAddon;
  private searchAddon?: SearchAddon;
  private resizeObserver?: ResizeObserver;
  private offHandlers: Array<() => void> = [];
  private sessionId = '';
  private disposed = false;
  private fitPending = false;

  @Input({ required: true }) tab!: TerminalTab;
  @Input() active = false;
  @Output() readonly ready = new EventEmitter<TerminalReady>();
  @Output() readonly exited = new EventEmitter<string>();
  @ViewChild('host', { static: true }) host!: ElementRef<HTMLDivElement>;

  readonly error = signal('');
  readonly searchOpen = signal(false);
  readonly searchTerm = signal('');
  readonly searchCount = signal('');

  ngAfterViewInit(): void {
    void this.boot();
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (changes['active']?.currentValue && this.terminal) {
      this.refit();
      requestAnimationFrame(() => this.terminal?.focus());
    }
  }

  ngOnDestroy(): void {
    this.disposed = true;
    this.resizeObserver?.disconnect();
    for (const off of this.offHandlers) off();
    this.offHandlers = [];
    if (this.sessionId && !(this.tab.attachId && this.tab.shared)) {
      void this.bridge
        .call(`${TERMINAL_SERVICE}.Close`, [{ id: this.sessionId }])
        .catch(() => undefined);
    }
    this.terminal?.dispose();
  }

  search(event: Event): void {
    const value = (event.target as HTMLInputElement).value;
    this.searchTerm.set(value);
    if (!value) {
      this.searchAddon?.clearDecorations();
      this.searchCount.set('');
      return;
    }
    this.searchAddon?.findNext(value, searchOptions());
  }

  searchKey(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault();
      this.closeSearch();
      return;
    }
    if (event.key === 'Enter') {
      event.preventDefault();
      event.shiftKey ? this.findPrevious() : this.findNext();
    }
  }

  findNext(): void {
    const term = this.searchTerm();
    if (term) this.searchAddon?.findNext(term, searchOptions());
  }

  findPrevious(): void {
    const term = this.searchTerm();
    if (term) this.searchAddon?.findPrevious(term, searchOptions());
  }

  closeSearch(): void {
    this.searchOpen.set(false);
    this.searchAddon?.clearDecorations();
    this.terminal?.focus();
  }

  private async boot(): Promise<void> {
    const fontMono =
      getComputedStyle(document.documentElement).getPropertyValue('--font-mono').trim() ||
      'Menlo, Monaco, monospace';
    const terminal = new Terminal({
      cursorBlink: true,
      fontFamily: fontMono,
      fontSize: 13,
      lineHeight: 1.2,
      scrollback: 5_000,
      allowProposedApi: true,
      theme: {
        background: '#0b0e11',
        foreground: '#e5e7eb',
        cursor: '#40c1c5',
        cursorAccent: '#0b0e11',
        selectionBackground: 'rgba(64,193,197,.3)',
        black: '#1f2937',
        red: '#f87171',
        green: '#34d399',
        yellow: '#fbbf24',
        blue: '#60a5fa',
        magenta: '#c084fc',
        cyan: '#22d3ee',
        white: '#e5e7eb',
        brightBlack: '#475569',
        brightRed: '#fca5a5',
        brightGreen: '#6ee7b7',
        brightYellow: '#fcd34d',
        brightBlue: '#93c5fd',
        brightMagenta: '#d8b4fe',
        brightCyan: '#67e8f9',
        brightWhite: '#f8fafc',
      },
    });
    this.terminal = terminal;

    const fit = new FitAddon();
    const search = new SearchAddon();
    this.fitAddon = fit;
    this.searchAddon = search;
    terminal.loadAddon(fit);
    terminal.loadAddon(new WebLinksAddon());
    terminal.loadAddon(search);
    terminal.loadAddon(new ClipboardAddon());
    terminal.loadAddon(new UnicodeGraphemesAddon());
    const graphemeVersion = terminal.unicode.versions.find((version) =>
      version.includes('graphemes'),
    );
    if (graphemeVersion) terminal.unicode.activeVersion = graphemeVersion;

    search.onDidChangeResults(({ resultIndex, resultCount }) => {
      this.searchCount.set(
        resultCount > 0 ? `${resultIndex + 1}/${resultCount}` : this.searchTerm() ? '0/0' : '',
      );
    });
    terminal.attachCustomKeyEventHandler((event) => {
      if (
        event.type === 'keydown' &&
        (event.metaKey || event.ctrlKey) &&
        event.key.toLocaleLowerCase('en-GB') === 'f'
      ) {
        this.searchOpen.set(true);
        return false;
      }
      return true;
    });
    terminal.onData((data) => {
      if (!this.sessionId) return;
      void this.bridge
        .call(`${TERMINAL_SERVICE}.Write`, [{ id: this.sessionId, data }])
        .catch((error: unknown) => this.setError(error));
    });

    terminal.open(this.host.nativeElement);
    try {
      const { WebglAddon } = await import('@xterm/addon-webgl');
      if (this.disposed) return;
      const webgl = new WebglAddon();
      webgl.onContextLoss(() => webgl.dispose());
      terminal.loadAddon(webgl);
    } catch {
      // DOM rendering is the supported fallback when WebGL2 is unavailable.
    }
    this.refit();

    try {
      const opened = this.tab.attachId
        ? { id: this.tab.attachId, cwd: this.tab.cwd ?? '', shell: this.tab.title }
        : normaliseOpen(
            await this.bridge.call(`${TERMINAL_SERVICE}.Open`, [
              {
                repo: this.tab.repo ?? '',
                cwd: this.tab.cwd ?? '',
                command: [...(this.tab.command ?? [])],
                term: 'xterm-256color',
                cols: terminal.cols,
                rows: terminal.rows,
              },
            ]),
          );
      if (!opened?.id || this.disposed) {
        throw new Error('The terminal service did not return a session id.');
      }
      this.sessionId = opened.id;
      await this.subscribe(opened.id);
      await this.bridge.call(`${TERMINAL_SERVICE}.Attach`, [{ id: opened.id }]);
      this.ready.emit({
        key: this.tab.key,
        title: this.tab.repo || basename(opened.cwd) || basename(opened.shell) || this.tab.title,
        cwd: opened.cwd,
        shell: opened.shell,
      });
      this.resizeObserver = new ResizeObserver(() => this.refit());
      this.resizeObserver.observe(this.host.nativeElement);
      if (this.active) terminal.focus();
    } catch (error) {
      this.setError(error);
    }
  }

  private async subscribe(id: string): Promise<void> {
    const { Events } = await import('@wailsio/runtime');
    this.offHandlers.push(
      Events.On(`lthn:term:out:${id}`, (event) => {
        if (typeof event.data !== 'string') return;
        try {
          this.terminal?.write(base64Bytes(event.data));
        } catch {
          this.error.set(
            $localize`:Terminal decode failure@@surface.agents.terminal.decodeFailure:Terminal output could not be decoded.`,
          );
        }
      }),
      Events.On(`lthn:term:exit:${id}`, () => {
        this.exited.emit(this.tab.key);
      }),
    );
  }

  private refit(): void {
    if (this.fitPending) return;
    this.fitPending = true;
    requestAnimationFrame(() => {
      this.fitPending = false;
      if (this.disposed || !this.terminal || !this.fitAddon) return;
      try {
        this.fitAddon.fit();
      } catch {
        return;
      }
      if (this.sessionId) {
        void this.bridge
          .call(`${TERMINAL_SERVICE}.Resize`, [
            {
              id: this.sessionId,
              cols: this.terminal.cols,
              rows: this.terminal.rows,
            },
          ])
          .catch(() => undefined);
      }
    });
  }

  private setError(error: unknown): void {
    this.error.set(error instanceof Error ? error.message : String(error));
  }
}

function searchOptions() {
  return {
    caseSensitive: false,
    regex: false,
    wholeWord: false,
    decorations: {
      matchBackground: '#4d3b00',
      matchBorder: '#7a5e00',
      matchOverviewRuler: '#7a5e00',
      activeMatchBackground: '#b3860b',
      activeMatchBorder: '#ffd966',
      activeMatchColorOverviewRuler: '#ffd966',
    },
  } as const;
}

function base64Bytes(value: string): Uint8Array {
  const binary = atob(value);
  return Uint8Array.from(binary, (character) => character.charCodeAt(0));
}

function basename(value: string): string {
  return value.split('/').filter(Boolean).at(-1) ?? '';
}

function normaliseOpen(value: unknown): {
  id: string;
  cwd: string;
  shell: string;
} | null {
  if (!value || typeof value !== 'object') return null;
  const record = value as Record<string, unknown>;
  const id = typeof record['id'] === 'string' ? record['id'] : '';
  if (!id) return null;
  return {
    id,
    cwd: typeof record['cwd'] === 'string' ? record['cwd'] : '',
    shell: typeof record['shell'] === 'string' ? record['shell'] : '',
  };
}
