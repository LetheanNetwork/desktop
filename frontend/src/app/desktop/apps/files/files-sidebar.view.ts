import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  input,
  output,
} from '@angular/core';
import type { FilesActionIntent, FilesViewState } from './files-view.models';

@Component({
  selector: 'lthn-files-sidebar-view',
  standalone: true,
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <aside
      class="fbside"
      aria-label="Files locations"
      i18n-aria-label="File browser locations@@files.sidebar.label"
    >
      <div class="slab" i18n="File browser sidebar group@@files.sidebar.favourites">Favourites</div>
      <button
        type="button"
        class="fbplace"
        data-token="home"
        [class.on]="state().location.kind === 'home'"
        [attr.aria-current]="state().location.kind === 'home' ? 'page' : null"
        (click)="navigate('home')"
      >
        <lthn-icon name="house" size="15"></lthn-icon>
        <span i18n="File browser place@@files.place.home">Home</span>
      </button>
      @for (mount of state().catalogue.mounts; track mount.id) {
        <button
          type="button"
          class="fbplace"
          [attr.data-token]="mount.id"
          [class.on]="activeMount(mount.id)"
          [attr.aria-current]="activeMount(mount.id) ? 'page' : null"
          (click)="navigate(mount.id)"
        >
          <lthn-icon [attr.name]="mount.icon || 'folder'" size="15"></lthn-icon>
          <span>{{ mount.name }}</span>
        </button>
      }

      <div class="slab" i18n="File browser sidebar group@@files.sidebar.locations">Locations</div>
      <button
        type="button"
        class="fbplace"
        data-token="trash"
        [class.on]="state().location.kind === 'trash'"
        [attr.aria-current]="state().location.kind === 'trash' ? 'page' : null"
        (click)="navigate('trash')"
      >
        <lthn-icon name="trash-can" size="15"></lthn-icon>
        <span i18n="File browser place@@files.place.trash">Trash</span>
      </button>
    </aside>
  `,
})
export class FilesSidebarView {
  readonly state = input.required<FilesViewState>();
  readonly intent = output<FilesActionIntent>();

  activeMount(mountId: string): boolean {
    const location = this.state().location;
    return location.kind === 'directory' && location.mountId === mountId;
  }

  navigate(token: string): void {
    this.intent.emit({ type: 'navigate', token });
  }
}
