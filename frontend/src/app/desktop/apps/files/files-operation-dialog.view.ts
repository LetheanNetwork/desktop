import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
  signal,
} from '@angular/core';
import type {
  FilesActionIntent,
  FilesCatalogueView,
  FilesOperationDialogView,
} from './files-view.models';

@Component({
  selector: 'lthn-files-operation-dialog-view',
  standalone: true,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="fbdialog-backdrop">
      <section
        class="fbdialog"
        role="dialog"
        aria-modal="true"
        [attr.aria-labelledby]="'files-operation-title'"
        [attr.aria-busy]="dialog().state === 'busy'"
      >
        <header>
          <h2 id="files-operation-title">{{ dialog().title }}</h2>
          @if (dialog().state !== 'busy') {
            <button
              type="button"
              data-action="dismiss"
              (click)="intent.emit({ type: 'dismiss-dialog' })"
              aria-label="Close"
              i18n-aria-label="Close Files operation dialog@@files.dialog.close"
            >
              <lthn-icon name="xmark" size="13"></lthn-icon>
            </button>
          }
        </header>

        <p [class.warning]="dialog().state === 'partial' || dialog().state === 'error'">
          {{ dialog().message }}
        </p>

        @if (dialog().state === 'form') {
          @if (dialog().operation === 'create-directory' || dialog().operation === 'rename') {
            <label>
              <span i18n="Files operation name field@@files.dialog.name">Name</span>
              <input
                #nameInput
                type="text"
                [value]="dialog().name ?? ''"
                (input)="name.set(nameInput.value)"
                autocomplete="off"
              />
            </label>
          }
          @if (dialog().operation === 'copy' || dialog().operation === 'move') {
            <label>
              <span i18n="Files destination mount field@@files.dialog.destinationMount">
                Destination
              </span>
              <select #mountInput (change)="destinationMountId.set(mountInput.value)">
                <option value="" i18n="Select mount prompt@@files.dialog.selectMount">
                  Select a location
                </option>
                @for (mount of catalogue()?.mounts ?? []; track mount.id) {
                  @if (mount.capabilities.copyTo) {
                    <option [value]="mount.id">{{ mount.name }}</option>
                  }
                }
              </select>
            </label>
            <label>
              <span i18n="Files destination path field@@files.dialog.destinationPath">
                Relative path
              </span>
              <input
                #pathInput
                type="text"
                [value]="dialog().destination?.path ?? ''"
                (input)="destinationPath.set(pathInput.value)"
                autocomplete="off"
                placeholder="folder/name"
              />
            </label>
          }
        }

        @if (dialog().state === 'busy') {
          <div class="fbdialog-progress">
            <lthn-icon name="spinner" size="18"></lthn-icon>
            <span i18n="Files operation busy state@@files.dialog.busy">Working…</span>
          </div>
        }

        @if (dialog().state === 'conflict' && dialog().result?.conflict; as conflict) {
          <dl class="fbdialog-addresses">
            <dt i18n="Files conflict source@@files.dialog.conflictSource">Source</dt>
            <dd>{{ conflict.source.mountId }} · {{ conflict.source.path }}</dd>
            <dt i18n="Files conflict destination@@files.dialog.conflictDestination">Destination</dt>
            <dd>{{ conflict.destination.mountId }} · {{ conflict.destination.path }}</dd>
          </dl>
        }

        <footer>
          @if (dialog().state === 'form') {
            <button
              type="button"
              data-action="submit"
              (click)="submit(false)"
              i18n="Continue Files operation@@files.dialog.continue"
            >
              Continue
            </button>
          } @else if (dialog().state === 'confirm') {
            <button
              type="button"
              class="danger"
              data-action="confirm"
              (click)="submit(true)"
              i18n="Confirm Files operation@@files.dialog.confirm"
            >
              Confirm
            </button>
          } @else if (dialog().state !== 'busy') {
            <button
              type="button"
              data-action="dismiss"
              (click)="intent.emit({ type: 'dismiss-dialog' })"
              i18n="Dismiss Files feedback@@files.dialog.dismiss"
            >
              Dismiss
            </button>
          }
        </footer>
      </section>
    </div>
  `,
})
export class FilesOperationDialogViewComponent {
  readonly dialog = input.required<FilesOperationDialogView>();
  readonly catalogue = input<FilesCatalogueView | null>(null);
  readonly intent = output<FilesActionIntent>();

  protected readonly name = signal('');
  protected readonly destinationMountId = signal('');
  protected readonly destinationPath = signal('');

  submit(confirmed: boolean): void {
    const dialog = this.dialog();
    const destinationMountId = this.destinationMountId() || dialog.destination?.mountId || '';
    const destinationPath = this.destinationPath() || dialog.destination?.path || '';
    this.intent.emit({
      type: 'submit-operation',
      operation: dialog.operation,
      ...(dialog.source ? { source: dialog.source } : {}),
      ...(destinationMountId
        ? {
            destination: {
              mountId: destinationMountId,
              path: destinationPath,
            },
          }
        : {}),
      ...(dialog.receiptId ? { receiptId: dialog.receiptId } : {}),
      ...(this.name() || dialog.name ? { name: this.name() || dialog.name } : {}),
      recursive: dialog.recursive ?? false,
      confirmed,
    });
  }
}
