import { CommonModule } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  ElementRef,
  ViewEncapsulation,
  input,
  output,
  viewChild,
} from '@angular/core';
import type { ShellAppRequest, ShellLanguage, ShellTrayKey, ShellWorldClock } from './shell.types';

@Component({
  selector: 'lthn-shell-tray-panel',
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './tray-panel.html',
  styleUrl: './tray-panel.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  host: {
    style: 'display:contents',
  },
})
export class ShellTrayPanel {
  readonly open = input.required<boolean>();
  readonly trayKey = input.required<ShellTrayKey | ''>();
  readonly left = input.required<number>();
  readonly top = input.required<number>();
  readonly languages = input.required<readonly ShellLanguage[]>();
  readonly language = input.required<string>();
  readonly clockText = input.required<string>();
  readonly dateText = input.required<string>();
  readonly worldClocks = input.required<readonly ShellWorldClock[]>();
  readonly wattsJson = input.required<string>();

  readonly languageRequested = output<string>();
  readonly appRequested = output<ShellAppRequest>();

  private readonly panel = viewChild<ElementRef<HTMLElement>>('panel');

  get panelElement(): HTMLElement | undefined {
    return this.panel()?.nativeElement;
  }
}
