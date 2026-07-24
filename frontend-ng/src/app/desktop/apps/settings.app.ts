import {
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  inject,
  ChangeDetectionStrategy,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { AppNavItem } from '../desktop-route-tree';
import { Win } from '../desktop.data';
import { PreferencesService } from '../preferences.service';
import { WindowManagerService } from '../window-manager.service';

@Component({
  selector: 'lthn-settings-app',
  standalone: true,
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.Eager,
  template: `
    <nav class="rail">
      <a
        *ngFor="let item of nav; let last = last"
        [class.on]="(win.sub || 'interface') === item[0]"
        [class.last]="last"
        (click)="wm.setSub(win.id, item[0])"
      >
        <lthn-icon [attr.name]="item[1]" [attr.aria-label]="item[2]" size="15"></lthn-icon>
      </a>
    </nav>
    <div class="appbody" [ngSwitch]="win.sub || 'interface'">
      <ng-container *ngSwitchCase="'translations'">
        <div class="ctoolbar">
          <h1 i18n="Translations settings heading@@settings.translations.heading">Translations</h1>
          <span
            class="cfgsrc"
            i18n="Translation file breadcrumb@@settings.translations.fileBreadcrumb"
            >locales/{{ prefs.lang() }}.yaml · corego/pkg/i18n</span
          >
        </div>
        <div class="i18n">
          <div class="i18nrow i18nhead">
            <span i18n="Translation key column@@settings.translations.column.key">Key</span
            ><span i18n="English source column@@settings.translations.column.english">English</span
            ><span>{{ prefs.lang().toUpperCase() }}</span>
          </div>
          <div class="i18nrow" *ngFor="let row of i18nKeys">
            <span class="i18nk">{{ row[0] }}</span>
            <span class="i18nen">{{ row[1] }}</span>
            <span class="i18ncell" contenteditable="true">{{ translated(row) }}</span>
          </div>
        </div>
        <p class="cfghint" i18n="Translation editor help@@settings.translations.help">
          <lthn-icon name="circle-info" size="11"></lthn-icon> Click a
          {{ prefs.lang().toUpperCase() }} cell to edit — writes to
          <code>locales/{{ prefs.lang() }}.yaml</code> via corego/pkg/i18n.
        </p>
      </ng-container>
      <ng-container *ngSwitchDefault>
        <div class="ctoolbar">
          <h1 i18n="UI preferences heading@@settings.preferences.heading">UI preferences</h1>
        </div>
        <div class="setgroup">
          <span class="glab" i18n="Settings group@@settings.group.appearance">Appearance</span>
          <div class="setrow">
            <div class="k" i18n="Theme preference@@settings.theme">
              Theme<small>Light mode applies to the desktop only</small>
            </div>
            <div class="prefseg" role="group">
              <button
                [class.on]="prefs.mode() === 'dark'"
                (click)="prefs.mode.set('dark')"
                i18n="Dark theme option@@settings.theme.dark"
              >
                Dark
              </button>
              <button
                [class.on]="prefs.mode() === 'light'"
                (click)="prefs.mode.set('light')"
                i18n="Light theme option@@settings.theme.light"
              >
                Light
              </button>
            </div>
          </div>
          <div class="setrow">
            <div class="k" i18n="Design preference@@settings.design">
              Design<small>Brand token ramp</small>
            </div>
            <div class="prefseg" role="group">
              <button
                [class.on]="prefs.design() === 'lethean'"
                (click)="prefs.design.set('lethean')"
              >
                <ng-container i18n="Lethean design option@@settings.design.lethean"
                  >Lethean</ng-container
                >
              </button>
              <button
                [class.on]="prefs.design() === 'custom'"
                (click)="prefs.design.set('custom')"
                i18n="Custom design option@@settings.design.custom"
              >
                Custom
              </button>
            </div>
          </div>
          <div class="themed" *ngIf="prefs.design() === 'custom'">
            <input
              class="cfgin"
              style="width:100%"
              [value]="prefs.customName()"
              (input)="setCustomName($event)"
              aria-label="Design name"
              i18n-aria-label="Custom design name input@@settings.design.nameLabel"
            />
            <div class="thlab" i18n="Accent colour label@@settings.design.accent">Accent</div>
            <div class="thswatches">
              <button
                class="thsw"
                *ngFor="let hue of hues"
                [class.on]="prefs.customHue() === hue[0]"
                [title]="hue[1]"
                [style.background]="'oklch(0.62 0.15 ' + hue[0] + ')'"
                (click)="prefs.customHue.set(hue[0])"
              ></button>
            </div>
            <div class="thlab" i18n="Generated colour ramp label@@settings.design.generatedRamp">
              Generated ramp
            </div>
            <div class="thramp">
              <span
                *ngFor="let shade of [200, 300, 400, 500, 600, 700]"
                [style.background]="'var(--brand-' + shade + ')'"
              ></span>
            </div>
          </div>
          <div class="setrow">
            <div class="k" i18n="Wallpaper preference@@settings.wallpaper">
              Wallpaper<small>Desktop background</small>
            </div>
            <div class="prefseg" role="group">
              <button
                *ngFor="let wallpaper of wallpapers"
                [class.on]="prefs.wallpaper() === wallpaper"
                (click)="prefs.wallpaper.set(wallpaper)"
              >
                {{ wallpaperLabel(wallpaper) }}
              </button>
            </div>
          </div>
        </div>
        <div class="setgroup">
          <span class="glab" i18n="Settings group@@settings.group.desktop">Desktop</span>
          <div class="setrow">
            <div class="k" i18n="Taskbar edge preference@@settings.taskbarEdge">
              Taskbar edge<small>Dock the taskbar to any screen edge</small>
            </div>
            <div class="prefseg" role="group">
              <button
                *ngFor="let edge of edges"
                [class.on]="prefs.bar() === edge"
                (click)="prefs.bar.set(edge)"
              >
                {{ edgeLabel(edge) }}
              </button>
            </div>
          </div>
          <div class="setrow">
            <div class="k" i18n="Desktop layout preference@@settings.layout">
              Layout<small>Floating windows, app shell, or device frame</small>
            </div>
            <div class="prefseg" role="group">
              <button
                [class.on]="wm.view() === 'desktop'"
                (click)="wm.setView('desktop')"
                i18n="Desktop layout option@@settings.layout.desktop"
              >
                Desktop
              </button>
              <button
                [class.on]="wm.view() === 'shell'"
                (click)="wm.setView('shell')"
                i18n="App shell layout option@@settings.layout.appShell"
              >
                App shell
              </button>
              <button
                [class.on]="wm.view() === 'device'"
                (click)="wm.setView('device')"
                i18n="Device layout option@@settings.layout.device"
              >
                Device
              </button>
            </div>
          </div>
          <div class="setrow">
            <div class="k" i18n="Device size preference@@settings.deviceSize">
              Device size<small>Small, large, or full</small>
            </div>
            <div class="prefseg" role="group">
              <button
                *ngFor="let device of devices"
                [class.on]="wm.device() === device"
                (click)="wm.setDevice(device)"
              >
                {{ deviceLabel(device) }}
              </button>
            </div>
          </div>
          <div class="setrow">
            <div class="k" i18n="Desktop icon preference@@settings.desktopIcons">
              Desktop icons<small>Show launcher tiles on the wallpaper</small>
            </div>
            <div class="prefseg" role="group">
              <button
                [class.on]="prefs.showIcons()"
                (click)="prefs.showIcons.set(true)"
                i18n="Enabled option@@common.on"
              >
                On
              </button>
              <button
                [class.on]="!prefs.showIcons()"
                (click)="prefs.showIcons.set(false)"
                i18n="Disabled option@@common.off"
              >
                Off
              </button>
            </div>
          </div>
          <div class="setrow">
            <div class="k" i18n="Desktop widgets preference@@settings.widgets">
              Widgets<small>Chart, world clock &amp; package status</small>
            </div>
            <div class="prefseg" role="group">
              <button
                [class.on]="prefs.showWidgets()"
                (click)="prefs.showWidgets.set(true)"
                i18n="Enabled option@@common.on"
              >
                On
              </button>
              <button
                [class.on]="!prefs.showWidgets()"
                (click)="prefs.showWidgets.set(false)"
                i18n="Disabled option@@common.off"
              >
                Off
              </button>
            </div>
          </div>
          <div class="setrow">
            <div class="k" i18n="Reduced motion preference@@settings.reduceMotion">
              Reduce motion<small>Disable dock magnify &amp; transitions</small>
            </div>
            <div class="prefseg" role="group">
              <button
                [class.on]="prefs.reduceMotion()"
                (click)="prefs.reduceMotion.set(true)"
                i18n="Enabled option@@common.on"
              >
                On
              </button>
              <button
                [class.on]="!prefs.reduceMotion()"
                (click)="prefs.reduceMotion.set(false)"
                i18n="Disabled option@@common.off"
              >
                Off
              </button>
            </div>
          </div>
        </div>
      </ng-container>
    </div>
  `,
})
export class SettingsApp implements AppView {
  @Input({ required: true }) win!: Win;
  @Input() nav: AppNavItem[] = [];

  readonly prefs = inject(PreferencesService);
  readonly wm = inject(WindowManagerService);
  readonly wallpapers = ['aurora', 'dusk', 'mist', 'graphite'] as const;
  readonly devices = ['small', 'large', 'full'] as const;
  readonly edges = ['top', 'right', 'bottom', 'left'] as const;
  readonly hues: [number, string][] = [
    [190, $localize`:Accent colour@@settings.accent.teal:Teal`],
    [305, $localize`:Accent colour@@settings.accent.viPurple:Vi purple`],
    [250, $localize`:Accent colour@@settings.accent.blue:Blue`],
    [150, $localize`:Accent colour@@settings.accent.green:Green`],
    [40, $localize`:Accent colour@@settings.accent.ember:Ember`],
    [340, $localize`:Accent colour@@settings.accent.magenta:Magenta`],
  ];
  readonly i18nKeys: [string, string][] = [
    ['menu.file', 'File'],
    ['menu.edit', 'Edit'],
    ['menu.view', 'View'],
    ['action.save', 'Save'],
    ['action.cancel', 'Cancel'],
    ['action.open', 'Open'],
    ['session.lock', 'Lock screen'],
    ['session.logout', 'Log out'],
    ['app.control', 'Control'],
    ['app.games', 'Games'],
    ['app.notepad', 'Notepad'],
  ];
  readonly translations: Record<string, Record<string, string>> = {
    fr: {
      'menu.file': 'Fichier',
      'menu.edit': 'Édition',
      'menu.view': 'Affichage',
      'action.save': 'Enregistrer',
    },
    de: {
      'menu.file': 'Datei',
      'menu.edit': 'Bearbeiten',
      'menu.view': 'Ansicht',
      'action.save': 'Speichern',
    },
    cy: {
      'menu.file': 'Ffeil',
      'menu.edit': 'Golygu',
      'menu.view': 'Golwg',
      'action.save': 'Cadw',
    },
    es: {
      'menu.file': 'Archivo',
      'menu.edit': 'Editar',
      'menu.view': 'Ver',
      'action.save': 'Guardar',
    },
    ja: { 'menu.file': 'ファイル', 'menu.edit': '編集', 'menu.view': '表示' },
  };

  setCustomName(event: Event): void {
    this.prefs.customName.set((event.target as HTMLInputElement).value);
  }

  wallpaperLabel(wallpaper: string): string {
    const labels: Record<string, string> = {
      aurora: $localize`:Wallpaper name@@settings.wallpaper.aurora:aurora`,
      dusk: $localize`:Wallpaper name@@settings.wallpaper.dusk:dusk`,
      mist: $localize`:Wallpaper name@@settings.wallpaper.mist:mist`,
      graphite: $localize`:Wallpaper name@@settings.wallpaper.graphite:graphite`,
    };
    return labels[wallpaper] ?? wallpaper;
  }

  edgeLabel(edge: string): string {
    const labels: Record<string, string> = {
      top: $localize`:Taskbar edge@@settings.edge.top:top`,
      right: $localize`:Taskbar edge@@settings.edge.right:right`,
      bottom: $localize`:Taskbar edge@@settings.edge.bottom:bottom`,
      left: $localize`:Taskbar edge@@settings.edge.left:left`,
    };
    return labels[edge] ?? edge;
  }

  deviceLabel(device: string): string {
    const labels: Record<string, string> = {
      small: $localize`:Device size@@settings.device.small:small`,
      large: $localize`:Device size@@settings.device.large:large`,
      full: $localize`:Device size@@settings.device.full:full`,
    };
    return labels[device] ?? device;
  }

  translated(row: [string, string]): string {
    const lang = this.prefs.lang();
    return lang === 'en' ? row[1] : (this.translations[lang]?.[row[0]] ?? '');
  }
}
