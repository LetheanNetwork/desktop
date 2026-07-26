import { CLOCKS, PACKAGE_STATUS_VARIANTS, PKGS } from './desktop-shell-fixtures.data';

describe('desktop shell fixtures', () => {
  it('provides unique clocks with valid IANA time zones', () => {
    const cities = CLOCKS.map((clock) => clock.city);
    const formatted = CLOCKS.map((clock) =>
      new Intl.DateTimeFormat('en-GB', {
        hour: '2-digit',
        minute: '2-digit',
        timeZone: clock.tz,
      }).format(new Date('2026-07-26T12:00:00Z')),
    );

    expect(new Set(cities).size).toBe(cities.length);
    expect(formatted.every((time) => time.length === 5)).toBe(true);
  });

  it('provides unique package rows with supported status variants', () => {
    const names = PKGS.map((pkg) => pkg.name);

    expect(new Set(names).size).toBe(names.length);
    expect(PKGS.every((pkg) => pkg.name.length > 0 && pkg.state.length > 0)).toBe(true);
    expect(PKGS.every((pkg) => PACKAGE_STATUS_VARIANTS.includes(pkg.variant))).toBe(true);
  });
});
