import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
} from '@angular/core';
import type { FilesActionIntent, FilesBrowserEntryView, FilesViewState } from './files-view.models';

@Component({
  selector: 'lthn-files-browser-view',
  standalone: true,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section
      class="fbbody"
      aria-label="Files"
      i18n-aria-label="File browser contents@@files.browser.label"
      [attr.aria-busy]="state().dataState === 'loading'"
    >
      @if (!state().entries.length) {
        <div class="fbempty">
          <lthn-icon
            [attr.name]="state().location.kind === 'trash' ? 'trash-can' : 'folder-open'"
            size="36"
          ></lthn-icon>
          <div>{{ state().emptyLabel }}</div>
        </div>
      } @else if (state().viewMode === 'list') {
        <div class="fblist" role="list">
          <div class="fbrow head" aria-hidden="true">
            <span i18n="File list column@@files.column.name">Name</span>
            <span i18n="File list column@@files.column.size">Size</span>
            <span i18n="File list column@@files.column.modified">Modified</span>
          </div>
          @for (entry of state().entries; track entryKey(entry)) {
            <button
              type="button"
              role="listitem"
              class="fbrow"
              [class.selected]="selected(entry)"
              [attr.data-path]="entry.relativePath"
              [attr.data-kind]="entry.kind"
              [attr.aria-pressed]="selected(entry)"
              [attr.aria-label]="entryLabel(entry)"
              (click)="select(entry)"
              (dblclick)="open(entry)"
              (keydown.enter)="open(entry)"
              (keydown.space)="selectWithKeyboard($event, entry)"
            >
              <span class="nm">
                <lthn-icon [attr.name]="entry.icon" size="15"></lthn-icon>
                <span>{{ entry.name }}</span>
              </span>
              <span class="mut">{{ entry.detail || '—' }}</span>
              <span class="mut">{{ entry.modifiedAt || '—' }}</span>
            </button>
          }
        </div>
      } @else {
        <div class="fbgrid" role="list">
          @for (entry of state().entries; track entryKey(entry)) {
            <button
              type="button"
              role="listitem"
              class="fbcell"
              [class.selected]="selected(entry)"
              [attr.data-path]="entry.relativePath"
              [attr.data-kind]="entry.kind"
              [attr.aria-pressed]="selected(entry)"
              [attr.aria-label]="entryLabel(entry)"
              (click)="select(entry)"
              (dblclick)="open(entry)"
              (keydown.enter)="open(entry)"
              (keydown.space)="selectWithKeyboard($event, entry)"
            >
              <lthn-icon
                class="fi"
                [class.folder]="entry.kind === 'directory'"
                [attr.name]="entry.icon"
                size="30"
              ></lthn-icon>
              <span class="fn">{{ entry.name }}</span>
              <span class="fc">{{ entry.detail }}</span>
            </button>
          }
        </div>
      }
    </section>
  `,
})
export class FilesBrowserView {
  readonly state = input.required<FilesViewState>();
  readonly selectedKey = input('');
  readonly intent = output<FilesActionIntent>();

  entryKey(entry: FilesBrowserEntryView): string {
    return `${entry.mountId}::${entry.relativePath}::${entry.receiptId}`;
  }

  selected(entry: FilesBrowserEntryView): boolean {
    return this.selectedKey() === this.entryKey(entry);
  }

  select(entry: FilesBrowserEntryView): void {
    this.intent.emit({
      type: 'select-entry',
      mountId: entry.mountId,
      path: entry.relativePath,
      receiptId: entry.receiptId,
    });
  }

  selectWithKeyboard(event: Event, entry: FilesBrowserEntryView): void {
    event.preventDefault();
    this.select(entry);
  }

  open(entry: FilesBrowserEntryView): void {
    if (entry.kind === 'directory') {
      this.intent.emit({
        type: 'open-directory',
        mountId: entry.mountId,
        path: entry.relativePath,
      });
      return;
    }
    if (entry.kind === 'file' && entry.available && this.state().location.kind !== 'trash') {
      this.intent.emit({
        type: 'preview',
        mountId: entry.mountId,
        path: entry.relativePath,
      });
    }
  }

  entryLabel(entry: FilesBrowserEntryView): string {
    const kind =
      entry.kind === 'directory'
        ? $localize`:File entry directory kind@@files.entry.directory:folder`
        : entry.kind === 'file'
          ? $localize`:File entry file kind@@files.entry.file:file`
          : $localize`:File entry unsupported kind@@files.entry.unsupported:unsupported item`;
    return `${entry.name}, ${kind}`;
  }
}
