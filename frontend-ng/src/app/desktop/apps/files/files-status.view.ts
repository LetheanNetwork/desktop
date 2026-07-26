import { ChangeDetectionStrategy, Component, CUSTOM_ELEMENTS_SCHEMA, input } from '@angular/core';
import type { FilesDataState, FilesViewState } from './files-view.models';

@Component({
  selector: 'lthn-files-status-view',
  standalone: true,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <footer class="fbstatus" [class.stale]="state().dataState === 'stale'">
      <lthn-badge
        [attr.variant]="badgeVariant(state().dataState)"
        [attr.data-data-state]="state().dataState"
      >
        {{ stateLabel(state().dataState) }}
      </lthn-badge>
      <span>{{ countLabel() }}</span>
      <span class="provider">{{ state().providerLabel }}</span>
      @if (state().capacityLabel) {
        <span class="v">{{ state().capacityLabel }}</span>
      }
    </footer>
  `,
})
export class FilesStatusView {
  readonly state = input.required<FilesViewState>();

  stateLabel(state: FilesDataState): string {
    switch (state) {
      case 'demo':
        return $localize`:Demo data state@@desktop.data.demo:Demo data`;
      case 'loading':
        return $localize`:Live data loading state@@desktop.data.loading:Loading live data`;
      case 'live':
        return $localize`:Live data state@@desktop.data.live:Live data`;
      case 'stale':
        return $localize`:Stale live Files data state@@files.data.stale:Live data stale`;
      case 'unavailable':
        return $localize`:Unavailable live Files data state@@files.data.unavailable:Live unavailable`;
    }
  }

  badgeVariant(state: FilesDataState): 'ok' | 'muted' | 'warn' {
    if (state === 'live') return 'ok';
    if (state === 'loading') return 'muted';
    return 'warn';
  }

  countLabel(): string {
    const state = this.state();
    const itemLabel =
      state.itemCount === 1
        ? $localize`:Singular item label@@files.count.item:item`
        : $localize`:Plural item label@@files.count.items:items`;
    const folderLabel =
      state.folderCount === 1
        ? $localize`:Singular folder label@@files.count.folder:folder`
        : $localize`:Plural folder label@@files.count.folders:folders`;
    const fileLabel =
      state.fileCount === 1
        ? $localize`:Singular file label@@files.count.file:file`
        : $localize`:Plural file label@@files.count.files:files`;
    return $localize`:File browser item summary@@files.status.summary:${state.itemCount}:itemCount: ${itemLabel}:itemLabel: · ${state.folderCount}:folderCount: ${folderLabel}:folderLabel:, ${state.fileCount}:fileCount: ${fileLabel}:fileLabel:`;
  }
}
