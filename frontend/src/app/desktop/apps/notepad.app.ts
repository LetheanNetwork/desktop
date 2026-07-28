// apps/notepad.app.ts — dumb app-view with local editor state (Sans/Mono, wrap,
// live Ln/Col + counts). The styling base for the future code editor.
import { Component, CUSTOM_ELEMENTS_SCHEMA, Input, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { Win } from '../desktop.data';

const SAMPLE = $localize`:Default notepad document@@notepad.sampleDocument:Lethean — scratch notes

The general text editor. Styling only for now; the code-editor
system (syntax, gutters, LSP) lands later and reuses this theme.

- Sans by default, Mono a click away
- Wrap toggles soft-wrap
- Live Ln / Col + word / char count below
`;

@Component({
  selector: 'lthn-notepad-app',
  standalone: true,
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.Eager,
  template: `
    <div class="editor">
      <div class="edtabs">
        <span class="edtab on" i18n="Untitled document tab@@notepad.untitledTab"
          ><lthn-icon name="file-lines" size="11"></lthn-icon> untitled.txt</span
        ><span class="edtools"
          ><span class="edseg"
            ><button
              [class.on]="!mono"
              (click)="mono = false"
              i18n="Sans-serif editor font@@notepad.font.sans"
            >
              Sans</button
            ><button
              [class.on]="mono"
              (click)="mono = true"
              i18n="Monospace editor font@@notepad.font.mono"
            >
              Mono
            </button></span
          ><button
            class="edwrap"
            [class.on]="wrap"
            (click)="wrap = !wrap"
            i18n="Editor line wrapping action@@notepad.wrap"
          >
            Wrap
          </button></span
        >
      </div>
      <textarea
        class="edarea"
        [class.mono]="mono"
        [class.nowrap]="!wrap"
        spellcheck="false"
        [value]="text"
        (input)="upd($event)"
        (keyup)="upd($event)"
        (mouseup)="upd($event)"
        placeholder="Start typing…"
        i18n-placeholder="Editor placeholder@@notepad.startTyping"
      ></textarea>
      <div class="edstatus">
        <span>{{ pos }}</span
        ><span i18n="Editor word and character counts@@notepad.wordCharacterCount"
          >{{ words }} words · {{ chars }} chars</span
        ><span class="edmeta" i18n="Editor encoding and document type@@notepad.documentMetadata"
          >UTF-8 · LF · Plain text</span
        >
      </div>
    </div>
  `,
})
export class NotepadApp implements AppView {
  @Input() win!: Win;
  @Input() text = SAMPLE;
  mono = false;
  wrap = true;
  pos = $localize`:Editor cursor position@@notepad.cursorPosition:Ln ${1}:line:, Col ${1}:column:`;
  words = (SAMPLE.trim().match(/\S+/g) || []).length;
  chars = SAMPLE.length;
  upd(ev: Event) {
    const ta = ev.target as HTMLTextAreaElement,
      v = ta.value,
      p = ta.selectionStart,
      b = v.slice(0, p);
    this.pos = $localize`:Editor cursor position@@notepad.cursorPosition:Ln ${b.split('\n').length}:line:, Col ${p - b.lastIndexOf('\n')}:column:`;
    this.words = (v.trim().match(/\S+/g) || []).length;
    this.chars = v.length;
  }
}
