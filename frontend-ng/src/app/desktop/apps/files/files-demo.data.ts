import type {
  FileEntryView,
  FileMountView,
  FilePreviewView,
  FileRecentView,
  FilesCapabilities,
} from './files-view.models';

export interface DemoDirectorySeed {
  readonly mountId: string;
  readonly path: string;
  readonly entries: readonly FileEntryView[];
}

export interface DemoHomeEntrySeed {
  readonly mountId: string;
  readonly entry: FileEntryView;
}

export interface DemoPreviewSeed extends FilePreviewView {}

const GIB = 1024 ** 3;
const MIB = 1024 ** 2;
const KIB = 1024;

const READ_WRITE: FilesCapabilities = {
  list: true,
  preview: true,
  createDirectory: true,
  write: true,
  rename: true,
  copyFrom: true,
  copyTo: true,
  move: true,
  trash: true,
  restore: true,
  delete: true,
};

const READ_ONLY: FilesCapabilities = {
  list: true,
  preview: true,
  createDirectory: false,
  write: false,
  rename: false,
  copyFrom: true,
  copyTo: false,
  move: false,
  trash: false,
  restore: false,
  delete: false,
};

const DEMO_CAPACITY = {
  freeBytes: 218 * GIB,
  totalBytes: 512 * GIB,
} as const;

export const DEMO_FILES_MOUNTS: readonly FileMountView[] = [
  {
    id: 'documents',
    name: $localize`:File browser place@@files.place.documents:Documents`,
    kind: 'memory',
    icon: 'folder',
    brand: false,
    capabilities: READ_WRITE,
    capacity: DEMO_CAPACITY,
  },
  {
    id: 'downloads',
    name: $localize`:File browser place@@files.place.downloads:Downloads`,
    kind: 'memory',
    icon: 'download',
    brand: false,
    capabilities: READ_WRITE,
    capacity: DEMO_CAPACITY,
  },
  {
    id: 'models',
    name: $localize`:File browser place@@files.place.models:Models`,
    kind: 'memory',
    icon: 'cube',
    brand: true,
    capabilities: READ_WRITE,
    capacity: DEMO_CAPACITY,
  },
  {
    id: 'projects',
    name: $localize`:File browser place@@files.place.projects:Projects`,
    kind: 'memory',
    icon: 'folder-tree',
    brand: false,
    capabilities: READ_WRITE,
    capacity: DEMO_CAPACITY,
  },
  {
    id: 'lethernet',
    name: $localize`:Application title@@app.lethernet.title:LetherNet`,
    kind: 'memory',
    icon: 'network-wired',
    brand: true,
    capabilities: READ_ONLY,
  },
];

export const DEMO_FILES_HOME_ENTRIES: readonly DemoHomeEntrySeed[] = [
  {
    mountId: 'documents',
    entry: file('welcome.txt', 'welcome.txt', 2 * KIB, '09:14'),
  },
  {
    mountId: 'documents',
    entry: file('lethean.png', 'lethean.png', 480 * KIB, 'Tue'),
  },
  {
    mountId: 'documents',
    entry: file('notes.md', 'notes.md', 6 * KIB, 'Mon'),
  },
];

export const DEMO_FILES_RECENT: readonly FileRecentView[] = DEMO_FILES_HOME_ENTRIES.map(
  ({ mountId, entry }) => ({
    mountId,
    path: entry.relativePath,
    name: entry.name,
    kind: entry.kind,
    openedAt: entry.modifiedAt,
  }),
);

export const DEMO_FILES_DIRECTORIES: readonly DemoDirectorySeed[] = [
  {
    mountId: 'documents',
    path: '',
    entries: [
      directory('Invoices', 'Invoices'),
      file('whitepaper.pdf', 'whitepaper.pdf', 2.4 * MIB, 'Jul 12'),
      file('roadmap.md', 'roadmap.md', 6 * KIB, 'Jul 18'),
      file('brand-guide.pdf', 'brand-guide.pdf', 8.1 * MIB, 'Jun 30'),
      file('meeting.txt', 'meeting.txt', 3 * KIB, 'Jul 20'),
    ],
  },
  {
    mountId: 'documents',
    path: 'Invoices',
    entries: [],
  },
  {
    mountId: 'downloads',
    path: '',
    entries: [
      file('core-1.20.tar.gz', 'core-1.20.tar.gz', 64 * MIB, 'Jul 21'),
      file('retroarch.dmg', 'retroarch.dmg', 112 * MIB, 'Jul 15'),
      file('gemma-2-27b.gguf', 'gemma-2-27b.gguf', 16 * GIB, 'Jul 10'),
      file('screenshot.png', 'screenshot.png', 1.2 * MIB, 'Today'),
    ],
  },
  {
    mountId: 'models',
    path: '',
    entries: [
      file('llama-3.1-70b.gguf', 'llama-3.1-70b.gguf', 40 * GIB, 'Jul 02'),
      file('mistral-small.gguf', 'mistral-small.gguf', 14 * GIB, 'Jun 28'),
      file('gemma-2-27b.gguf', 'gemma-2-27b.gguf', 16 * GIB, 'Jul 10'),
      file('phi-3-mini.gguf', 'phi-3-mini.gguf', 2.3 * GIB, 'May 19'),
      file('manifest.yaml', 'manifest.yaml', KIB, 'Jul 10'),
    ],
  },
  {
    mountId: 'projects',
    path: '',
    entries: [directory('lethean', 'lethean'), directory('core-ide', 'core-ide')],
  },
  {
    mountId: 'projects',
    path: 'lethean',
    entries: [
      file('go.work', 'lethean/go.work', KIB, 'Jul 22'),
      file('README.md', 'lethean/README.md', 4 * KIB, 'Jul 19'),
      file('main.go', 'lethean/main.go', 12 * KIB, 'Jul 22'),
      file('desktop.component.ts', 'lethean/desktop.component.ts', 18 * KIB, 'Today'),
      file('logo.svg', 'lethean/logo.svg', 12 * KIB, 'Jun 01'),
    ],
  },
  {
    mountId: 'projects',
    path: 'core-ide',
    entries: [
      file('angular.json', 'core-ide/angular.json', 3 * KIB, 'Jul 12'),
      file('package.json', 'core-ide/package.json', 2 * KIB, 'Jul 12'),
      file('app.routes.ts', 'core-ide/app.routes.ts', 9 * KIB, 'Jul 20'),
      file('bundle.zip', 'core-ide/bundle.zip', 24 * MIB, 'Jul 18'),
    ],
  },
  {
    mountId: 'lethernet',
    path: '',
    entries: [
      file('peers.yaml', 'peers.yaml', KIB, '09:16'),
      file('vi-01.log', 'vi-01.log', 220 * KIB, 'live'),
      file('vi-02.log', 'vi-02.log', 180 * KIB, 'live'),
      file('pool-manifest.json', 'pool-manifest.json', 2 * KIB, '09:12'),
    ],
  },
];

export const DEMO_FILES_PREVIEWS: readonly DemoPreviewSeed[] = [
  textPreview(
    'documents',
    'welcome.txt',
    'Welcome to Lethean Desktop.\n\nThis is deterministic demo data.',
    'text/plain',
  ),
  textPreview(
    'documents',
    'notes.md',
    '# Notes\n\nThe Files preview is bounded and provider-neutral.',
    'text/markdown',
  ),
  textPreview(
    'documents',
    'roadmap.md',
    '# Roadmap\n\n- Medium-backed Files\n- Provider-native watch events',
    'text/markdown',
  ),
  textPreview(
    'documents',
    'meeting.txt',
    'Meeting notes\n\nKeep the interface calm and useful.',
    'text/plain',
  ),
  textPreview(
    'projects',
    'lethean/README.md',
    '# Lethean Desktop\n\nAngular UI hosted by Wails and CoreGO.',
    'text/markdown',
  ),
  textPreview(
    'projects',
    'lethean/main.go',
    'package main\n\nfunc main() {\n\t// Demo preview.\n}\n',
    'text/plain',
  ),
  textPreview(
    'models',
    'manifest.yaml',
    'models:\n  - llama-3.1-70b\n  - mistral-small\n',
    'text/yaml',
  ),
  textPreview('lethernet', 'peers.yaml', 'peers:\n  - vi-01\n  - vi-02\n', 'text/yaml'),
];

function file(
  name: string,
  relativePath: string,
  sizeBytes: number,
  modifiedAt: string,
): FileEntryView {
  return {
    name,
    relativePath,
    kind: 'file',
    sizeBytes: Math.round(sizeBytes),
    modifiedAt,
    mode: 0o644,
    hidden: false,
  };
}

function directory(name: string, relativePath: string): FileEntryView {
  return {
    name,
    relativePath,
    kind: 'directory',
    sizeBytes: 0,
    modifiedAt: '',
    mode: 0o755,
    hidden: false,
  };
}

function textPreview(
  mountId: string,
  relativePath: string,
  content: string,
  mime: string,
): DemoPreviewSeed {
  return {
    mountId,
    relativePath,
    name: relativePath.split('/').at(-1) ?? relativePath,
    content,
    mime,
    bytesRead: new TextEncoder().encode(content).byteLength,
    sizeBytes: new TextEncoder().encode(content).byteLength,
    lines: content.split('\n').length,
    truncated: false,
    binary: false,
  };
}
