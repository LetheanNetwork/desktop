import { ChangeDetectionStrategy, Component } from '@angular/core';
import { FilesApp } from '../../apps/files.app';
import { SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-office-files-surface',
  standalone: true,
  imports: [FilesApp],
  template: `<lthn-files-app [win]="win" />`,
  host: { style: 'display:contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class OfficeFilesSurface extends SurfaceRoute {}
