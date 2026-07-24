import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-planning-calendar-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class PlanningCalendarSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'planning-calendar',
    title: $localize`:Planning calendar title@@surface.planning.calendar.title:Calendar`,
    subtitle: $localize`:Planning calendar subtitle@@surface.planning.calendar.subtitle:Week of 12 May`,
    icon: 'calendar-week',
    kind: 'calendar',
    metrics: [
      { label: 'Mon 12', value: '4', hint: 'events' },
      { label: 'Tue 13', value: '3', hint: 'events' },
      { label: 'Wed 14', value: '2', hint: 'events' },
      { label: 'Thu 15', value: '2', hint: 'events' },
      { label: 'Fri 16', value: '3', hint: 'today', tone: 'brand' },
    ],
    filters: [
      { id: 'focus', label: 'Focus' },
      { id: 'brand', label: 'Meetings' },
      { id: 'ship', label: 'Shipping' },
      { id: 'mute', label: 'Routine' },
    ],
    rows: [
      { id: 'mon-09', title: 'Standup', meta: 'Mon · 09:00', value: '1 h', tags: ['mute'] },
      {
        id: 'mon-10',
        title: 'Deep work · model browser',
        meta: 'Mon · 10:00',
        value: '2 h',
        tags: ['focus'],
      },
      {
        id: 'mon-14',
        title: 'Calliope VC call',
        meta: 'Mon · 14:00',
        value: '1 h',
        tags: ['brand'],
      },
      { id: 'mon-16', title: 'v0.2 release', meta: 'Mon · 16:00', value: '1 h', tags: ['ship'] },
      {
        id: 'tue-10',
        title: 'Deep work · LoRA wizard',
        meta: 'Tue · 10:00',
        value: '3 h',
        tags: ['focus'],
      },
      {
        id: 'wed-11',
        title: 'Sprint 24 planning',
        meta: 'Wed · 11:00',
        value: '2 h',
        tags: ['brand'],
      },
      {
        id: 'thu-10',
        title: 'Deep work · release notes',
        meta: 'Thu · 10:00',
        value: '3 h',
        tags: ['focus'],
      },
      {
        id: 'fri-13',
        title: 'Retro · Sprint 24',
        meta: 'Fri · 13:00',
        value: '2 h',
        tags: ['brand'],
      },
      { id: 'fri-16', title: 'Friday demo', meta: 'Fri · 16:00', value: '1 h', tags: ['ship'] },
    ],
    footer: 'focus blocks · ⌘B to add · ⌘⇧R to find a free hour · syncs locally only',
  };
}
