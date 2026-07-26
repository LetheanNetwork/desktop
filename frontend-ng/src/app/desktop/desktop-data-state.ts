export type DesktopDataState = 'demo' | 'loading' | 'live' | 'mixed' | 'unavailable';

export function desktopDataStateLabel(state: DesktopDataState): string {
  switch (state) {
    case 'demo':
      return $localize`:Demo data state@@desktop.data.demo:Demo data`;
    case 'loading':
      return $localize`:Live data loading state@@desktop.data.loading:Loading live data`;
    case 'live':
      return $localize`:Live data state@@desktop.data.live:Live data`;
    case 'mixed':
      return $localize`:Mixed live and demo data state@@desktop.data.mixed:Live + demo`;
    case 'unavailable':
      return $localize`:Live data unavailable state@@desktop.data.unavailable:Live unavailable · demo shown`;
  }
}

export function desktopDataStateVariant(state: DesktopDataState): 'ok' | 'muted' | 'warn' {
  if (state === 'live') return 'ok';
  if (state === 'loading') return 'muted';
  return 'warn';
}
