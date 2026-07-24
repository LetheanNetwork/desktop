// index.ts — public surface of the desktop kit. Import from here:
//   import { WindowManagerService, APP_REGISTRY, DESKTOP_DATA } from '@lethean/desktop';
export * from './desktop.data';
export * from './window-manager.service';
export * from './preferences.service';
export * from './apps/app-view';
export { TelemetryApp } from './apps/telemetry.app';
export { ActivityApp } from './apps/activity.app';
export { ChatApp } from './apps/chat.app';
export { LetherNetApp } from './apps/lethernet.app';
export { NotepadApp } from './apps/notepad.app';
export { FilesApp } from './apps/files.app';
export { GamesApp } from './apps/games.app';
export { ControlApp } from './apps/control.app';
export { DevPanelApp } from './apps/dev-panel.app';
export { DesktopComponent } from './desktop.component';
