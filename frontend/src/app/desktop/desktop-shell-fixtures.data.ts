export interface WorldClockFixture {
  readonly city: string;
  readonly zone: string;
  readonly tz: string;
}

export const PACKAGE_STATUS_VARIANTS = ['', 'ok'] as const;
export type PackageStatusVariant = (typeof PACKAGE_STATUS_VARIANTS)[number];

export interface PackageFixture {
  readonly name: string;
  readonly state: string;
  readonly variant: PackageStatusVariant;
}

export const CLOCKS = [
  {
    city: $localize`:World clock city@@desktop.clock.london:London`,
    zone: $localize`:World clock zone@@desktop.clock.londonZone:London`,
    tz: 'Europe/London',
  },
  {
    city: $localize`:World clock city@@desktop.clock.newYork:New York`,
    zone: $localize`:World clock zone@@desktop.clock.newYorkZone:New York`,
    tz: 'America/New_York',
  },
  {
    city: $localize`:World clock city@@desktop.clock.singapore:Singapore`,
    zone: $localize`:World clock zone@@desktop.clock.singaporeZone:Singapore`,
    tz: 'Asia/Singapore',
  },
] as const satisfies readonly WorldClockFixture[];

export const PKGS = [
  {
    name: 'llama.cpp',
    state: $localize`:Package state@@desktop.package.running:running`,
    variant: 'ok',
  },
  {
    name: 'lthn-runner',
    state: $localize`:Package state@@desktop.package.active:active`,
    variant: 'ok',
  },
  {
    name: 'LetherNet',
    state: $localize`:Package state@@desktop.package.idle:idle`,
    variant: '',
  },
] as const satisfies readonly PackageFixture[];
