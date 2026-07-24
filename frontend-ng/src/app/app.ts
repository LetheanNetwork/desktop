import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { Title } from '@angular/platform-browser';
import { RouterOutlet } from '@angular/router';
import { AppShell } from './app-shell/app-shell';

@Component({
  selector: 'app-root',
  imports: [RouterOutlet, AppShell],
  templateUrl: './app.html',
  styleUrl: './app.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class App {
  private readonly title = inject(Title);

  constructor() {
    this.title.setTitle($localize`:Application document title@@app.documentTitle:Lethean Desktop`);
  }
}
