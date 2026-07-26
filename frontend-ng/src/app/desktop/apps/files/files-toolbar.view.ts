import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
} from '@angular/core';
import type {
  FileAddressView,
  FilesActionIntent,
  FilesBrowserEntryView,
  FilesOperationKind,
  FilesViewState,
} from './files-view.models';

@Component({
  selector: 'lthn-files-toolbar-view',
  standalone: true,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="fbtop">
      <div class="fbnav">
        <button
          type="button"
          data-action="up"
          [disabled]="!state().upToken"
          (click)="intent.emit({ type: 'up' })"
          aria-label="Up"
          i18n-aria-label="Navigate to parent folder@@files.action.up"
        >
          <lthn-icon name="arrow-up" size="13"></lthn-icon>
        </button>
        <button
          type="button"
          data-action="home"
          [disabled]="state().location.kind === 'home'"
          (click)="intent.emit({ type: 'home' })"
          aria-label="Home"
          i18n-aria-label="Navigate to home folder@@files.action.home"
        >
          <lthn-icon name="house" size="13"></lthn-icon>
        </button>
        <button
          type="button"
          data-action="refresh"
          (click)="intent.emit({ type: 'refresh' })"
          aria-label="Refresh"
          i18n-aria-label="Refresh file browser@@files.action.refresh"
        >
          <lthn-icon name="rotate" size="13"></lthn-icon>
        </button>
      </div>

      <nav
        class="fbcrumb"
        aria-label="Files breadcrumbs"
        i18n-aria-label="File browser breadcrumbs@@files.breadcrumbs.label"
      >
        @for (
          crumb of state().breadcrumbs;
          track crumb.token;
          let first = $first;
          let last = $last
        ) {
          @if (!first) {
            <span class="sep" aria-hidden="true">/</span>
          }
          <button
            type="button"
            class="cr"
            [class.here]="last"
            [attr.data-token]="crumb.token"
            [attr.aria-current]="last ? 'page' : null"
            (click)="intent.emit({ type: 'navigate', token: crumb.token })"
          >
            {{ crumb.label }}
          </button>
        }
      </nav>

      <div class="fbactions">
        @if (state().location.kind === 'directory' && state().capabilities.createDirectory) {
          <button
            type="button"
            data-action="create-directory"
            (click)="openOperation('create-directory')"
            i18n="Create folder action@@files.action.createFolder"
          >
            Create folder
          </button>
        }
        @if (selection(); as entry) {
          @if (state().location.kind === 'trash') {
            @if (state().capabilities.restore && entry.available) {
              <button
                type="button"
                data-action="restore"
                (click)="openOperation('restore')"
                i18n="Restore file action@@files.action.restore"
              >
                Restore
              </button>
            }
            @if (state().capabilities.delete) {
              <button
                type="button"
                class="danger"
                data-action="delete"
                (click)="openOperation('delete')"
                i18n="Delete file permanently action@@files.action.delete"
              >
                Delete
              </button>
            }
          } @else {
            @if (state().capabilities.rename) {
              <button
                type="button"
                data-action="rename"
                (click)="openOperation('rename')"
                i18n="Rename file action@@files.action.rename"
              >
                Rename
              </button>
            }
            @if (state().capabilities.copyFrom) {
              <button
                type="button"
                data-action="copy"
                (click)="openOperation('copy')"
                i18n="Copy file action@@files.action.copy"
              >
                Copy
              </button>
            }
            @if (state().capabilities.move) {
              <button
                type="button"
                data-action="move"
                (click)="openOperation('move')"
                i18n="Move file action@@files.action.move"
              >
                Move
              </button>
            }
            @if (state().capabilities.trash) {
              <button
                type="button"
                class="danger"
                data-action="trash"
                (click)="openOperation('trash')"
                i18n="Move file to Trash action@@files.action.trash"
              >
                Trash
              </button>
            }
          }
        }
      </div>

      <div class="fbvtog">
        <button
          type="button"
          data-action="grid"
          [class.on]="state().viewMode === 'grid'"
          [attr.aria-pressed]="state().viewMode === 'grid'"
          (click)="intent.emit({ type: 'set-view', view: 'grid' })"
          aria-label="Grid view"
          i18n-aria-label="File grid view action@@files.action.gridView"
        >
          <lthn-icon name="table-cells-large" size="12"></lthn-icon>
        </button>
        <button
          type="button"
          data-action="list"
          [class.on]="state().viewMode === 'list'"
          [attr.aria-pressed]="state().viewMode === 'list'"
          (click)="intent.emit({ type: 'set-view', view: 'list' })"
          aria-label="List view"
          i18n-aria-label="File list view action@@files.action.listView"
        >
          <lthn-icon name="list" size="12"></lthn-icon>
        </button>
      </div>
    </header>
  `,
})
export class FilesToolbarView {
  readonly state = input.required<FilesViewState>();
  readonly selection = input<FilesBrowserEntryView | null>(null);
  readonly intent = output<FilesActionIntent>();

  openOperation(operation: FilesOperationKind): void {
    const selection = this.selection();
    const source: FileAddressView | undefined =
      operation === 'create-directory'
        ? this.currentDirectory()
        : selection
          ? {
              mountId: selection.mountId,
              path: selection.relativePath,
            }
          : undefined;
    this.intent.emit({
      type: 'open-operation',
      operation,
      ...(source ? { source } : {}),
      ...(selection?.receiptId ? { receiptId: selection.receiptId } : {}),
      ...(selection?.kind === 'directory' ? { recursive: true } : {}),
    });
  }

  private currentDirectory(): FileAddressView | undefined {
    const location = this.state().location;
    return location.kind === 'directory'
      ? { mountId: location.mountId, path: location.path }
      : undefined;
  }
}
