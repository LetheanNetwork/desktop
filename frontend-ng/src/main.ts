/// <reference types="@angular/localize" />

import { bootstrapApplication } from '@angular/platform-browser';
import { App } from './app/app';
import { appConfig } from './app/app.config';

// Development binaries expose Wails' build-tagged MCP service directly.
// The former event/WebMCP shim remains in wails-bridge.ts as an unbootstrapped
// compatibility fallback, but it no longer owns the application transport.
bootstrapApplication(App, appConfig).catch((err) => console.error(err));
