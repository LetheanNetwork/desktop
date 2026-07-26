import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
} from '@angular/core';
import type { FilePreviewView, FilesActionIntent } from './files-view.models';
import { formatBytes } from './files-view-state';

@Component({
  selector: 'lthn-files-preview-view',
  standalone: true,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <aside
      class="fbpreview"
      aria-label="File preview"
      i18n-aria-label="File preview panel@@files.preview.label"
    >
      <header>
        <div>
          <strong>{{ preview().name }}</strong>
          <span>{{ preview().mime }}</span>
        </div>
        <button
          type="button"
          (click)="intent.emit({ type: 'close-preview' })"
          aria-label="Close preview"
          i18n-aria-label="Close file preview@@files.preview.close"
        >
          <lthn-icon name="xmark" size="13"></lthn-icon>
        </button>
      </header>
      <div class="fbpreview-meta">
        <span>{{ formatBytes(preview().sizeBytes) }}</span>
        <span>
          {{ preview().bytesRead }}
          <ng-container i18n="Preview bytes read@@files.preview.bytesRead">
            bytes read
          </ng-container>
        </span>
        @if (!preview().binary) {
          <span>
            {{ preview().lines }}
            <ng-container i18n="Preview line count@@files.preview.lines">lines</ng-container>
          </span>
        }
      </div>
      @if (preview().binary) {
        <div class="fbpreview-unsupported">
          <lthn-icon name="file-shield" size="30"></lthn-icon>
          <strong i18n="Binary preview heading@@files.preview.binary"> Binary preview </strong>
          <span i18n="Binary preview message@@files.preview.binaryMessage">
            Metadata is shown without opening binary content.
          </span>
        </div>
      } @else {
        <pre>{{ preview().content }}</pre>
        @if (preview().truncated) {
          <p class="fbpreview-warning" i18n="Truncated preview warning@@files.preview.truncated">
            Preview truncated at the safe read limit.
          </p>
        }
      }
    </aside>
  `,
})
export class FilesPreviewView {
  readonly preview = input.required<FilePreviewView>();
  readonly intent = output<FilesActionIntent>();
  protected readonly formatBytes = formatBytes;
}
