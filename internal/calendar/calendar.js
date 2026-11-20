// Static EventKit bridge for Apple Calendar. Never interpolate input into
// this file: all input arrives as decoded JSON in `a` (see osa.Prelude).
ObjC.import('EventKit');
ObjC.import('AppKit');

const ENTITY_EVENT = 0;
const FULL_ACCESS = 3;
const WRITE_ONLY = 4;
const DAY_MS = 86400000;
const STATUS = ['none', 'confirmed', 'tentative', 'canceled'];
const FREQ = { daily: 0, weekly: 1, monthly: 2, yearly: 3 };
const SPAN = { this: 0, future: 1 };
// The search text is only ever an argument of this constant format.
const SEARCH_FORMAT = 'title CONTAINS[c] %@ OR location CONTAINS[c] %@ OR notes CONTAINS[c] %@';

function isNil(x) {
  return x === undefined || x === null || (typeof x.isNil === 'function' && x.isNil());
}
function str(x) { return isNil(x) ? '' : ObjC.unwrap(x); }
function ms(d) { return isNil(d) ? null : Math.round(d.timeIntervalSince1970 * 1000); }
function date(v) { return $.NSDate.dateWithTimeIntervalSince1970(v / 1000); }
function has(v) { return v !== null && v !== undefined; }
function items(arr) {
  const out = [];
  if (isNil(arr)) return out;
  const n = Number(arr.count);
  for (let i = 0; i < n; i++) out.push(arr.objectAtIndex(i));
  return out;
}

function waitFor(state, what) {
  const t0 = Date.now();
  while (!state.done) {
    if (Date.now() - t0 > 30000) fail('unavailable', 'timed out ' + what);
    $.NSRunLoop.currentRunLoop.runUntilDate($.NSDate.dateWithTimeIntervalSinceNow(0.01));
  }
}

function store() {
  let status = Number($.EKEventStore.authorizationStatusForEntityType(ENTITY_EVENT));
  const s = $.EKEventStore.alloc.init;
  if (status === 0) {
    const st = { done: false };
    s.requestFullAccessToEventsWithCompletion(function () { st.done = true; });
    waitFor(st, 'waiting for Calendars permission');
    status = Number($.EKEventStore.authorizationStatusForEntityType(ENTITY_EVENT));
  }
  if (status === WRITE_ONLY) {
    fail('permission', 'Calendars access is write-only; choose Full Access for your terminal app under Privacy & Security > Calendars');
  }
  if (status !== FULL_ACCESS) {
    fail('permission', 'Calendars access not granted (status ' + status + '); enable your terminal app under Privacy & Security > Calendars');
  }
  return s;
}

function check(ok, err, what) {
  if (ok) return;
  let m = '';
  try { m = str(err[0].localizedDescription); } catch (e) { m = ''; }
  fail('internal', what + ' failed' + (m ? ': ' + m : ''));
}

function hex(c) {
  try {
    const rgb = c.color.colorUsingColorSpace($.NSColorSpace.sRGBColorSpace);
    if (isNil(rgb)) return '';
    const h = function (v) { return ('0' + Math.round(Number(v) * 255).toString(16)).slice(-2); };
    return '#' + h(rgb.redComponent) + h(rgb.greenComponent) + h(rgb.blueComponent);
  } catch (e) {
    return '';
  }
}

function calendarOut(c) {
  return {
    id: str(c.calendarIdentifier), title: str(c.title), account: str(c.source.title),
    writable: !!c.allowsContentModifications, color: hex(c),
  };
}

// seriesId strips the "/RID=…" suffix EventKit gives detached occurrences,
// so an occurrence keeps its id after a span=this edit.
function seriesId(e) {
  const id = str(e.eventIdentifier);
  const i = id.indexOf('/RID=');
  return i < 0 ? id : id.slice(0, i);
}

function occurrence(e) {
  const o = ms(e.occurrenceDate);
  return o === null ? ms(e.startDate) : o;
}

function eventOut(e) {
  const org = e.organizer;
  return {
    event_id: seriesId(e),
    occurrence_ms: occurrence(e),
    title: str(e.title),
    calendar: str(e.calendar.title),
    calendar_id: str(e.calendar.calendarIdentifier),
    start_ms: ms(e.startDate),
    end_ms: ms(e.endDate),
    all_day: !!e.allDay,
    location: str(e.location),
    notes: str(e.notes),
    url: isNil(e.URL) ? '' : str(e.URL.absoluteString),
    recurring: !!e.hasRecurrenceRules || !!e.isDetached,
    status: STATUS[Number(e.status)] || 'none',
    organizer: isNil(org) ? '' : str(org.name),
    attendees: isNil(e.attendees) ? 0 : Number(e.attendees.count),
  };
}

function calendarById(s, id) {
  const c = s.calendarWithIdentifier(id);
  if (isNil(c) || (Number(c.allowedEntityTypes) & 1) === 0) fail('not_found', 'no calendar with id ' + id);
  return c;
}

function fetch(s, fromMs, toMs, cals) {
  return s.eventsMatchingPredicate(s.predicateForEventsWithStartDateEndDateCalendars(date(fromMs), date(toMs), cals));
}

// eventById resolves "<eventIdentifier>" plus the occurrence start: a
// non-recurring event is unique by identifier; a recurring occurrence is
// found in a window around its original occurrence date in its calendar.
function eventById(s, eid, occ) {
  const ev = s.eventWithIdentifier(eid);
  if (isNil(ev)) fail('not_found', 'no event with id ' + eid);
  if (!ev.hasRecurrenceRules) return ev;
  if (!has(occ)) fail('usage', 'recurring event: pass the occurrence id (<id>@<ms>) from events or search');
  const cals = $.NSMutableArray.array;
  cals.addObject(ev.calendar);
  const found = items(fetch(s, occ - 7 * DAY_MS, occ + 7 * DAY_MS, cals)).filter(function (e) {
    return seriesId(e) === eid && occurrence(e) === occ;
  });
  if (found.length === 0) fail('not_found', 'no occurrence of ' + eid + ' at ' + occ);
  return found[0];
}

function setTimes(e, startMs, endMs, allDay) {
  if (has(allDay)) e.allDay = !!allDay;
  if (has(startMs)) e.startDate = date(startMs);
  if (has(endMs)) e.endDate = date(endMs);
}

function save(s, e, span) {
  const err = Ref();
  check(s.saveEventSpanCommitError(e, span, true, err), err, 'saving the event');
}

const OPS = {
  calendars: function () {
    return items(store().calendarsForEntityType(ENTITY_EVENT)).map(calendarOut);
  },
  events: function (a) {
    const s = store();
    let cals = $();
    if (a.calendar_ids && a.calendar_ids.length) {
      cals = $.NSMutableArray.array;
      a.calendar_ids.forEach(function (id) { cals.addObject(calendarById(s, id)); });
    }
    let res = fetch(s, a.from_ms, a.to_ms, cals);
    if (a.query) {
      res = res.filteredArrayUsingPredicate($.NSPredicate.predicateWithFormatArgumentArray(SEARCH_FORMAT, [a.query, a.query, a.query]));
    }
    return items(res).map(eventOut);
  },
  get: function (a) {
    return eventOut(eventById(store(), a.event_id, a.occurrence_ms));
  },
  add: function (a) {
    const s = store();
    const e = $.EKEvent.eventWithEventStore(s);
    e.calendar = calendarById(s, a.calendar_id);
    e.title = a.title;
    setTimes(e, a.start_ms, a.end_ms, a.all_day);
    if (a.location) e.location = a.location;
    if (a.notes) e.notes = a.notes;
    if (a.url) e.URL = $.NSURL.URLWithString(a.url);
    if (a.repeat) {
      if (!has(FREQ[a.repeat])) fail('usage', 'unknown repeat frequency');
      const end = a.repeat_count > 0 ? $.EKRecurrenceEnd.recurrenceEndWithOccurrenceCount(a.repeat_count) : $();
      e.addRecurrenceRule($.EKRecurrenceRule.alloc.initRecurrenceWithFrequencyIntervalEnd(FREQ[a.repeat], 1, end));
    }
    save(s, e, SPAN.this);
    return eventOut(e);
  },
  update: function (a) {
    const s = store();
    const e = eventById(s, a.event_id, a.occurrence_ms);
    if (has(a.title)) e.title = a.title;
    if (has(a.location)) e.location = a.location;
    if (has(a.notes)) e.notes = a.notes;
    if (has(a.url)) e.URL = a.url === '' ? $() : $.NSURL.URLWithString(a.url);
    if (a.calendar_id) e.calendar = calendarById(s, a.calendar_id);
    setTimes(e, a.start_ms, a.end_ms, null);
    save(s, e, SPAN[a.span] || SPAN.this);
    return eventOut(e);
  },
  delete: function (a) {
    const s = store();
    const e = eventById(s, a.event_id, a.occurrence_ms);
    const err = Ref();
    check(s.removeEventSpanCommitError(e, SPAN[a.span] || SPAN.this, true, err), err, 'deleting the event');
    return { event_id: a.event_id };
  },
  calendar_create: function (a) {
    const s = store();
    const c = $.EKCalendar.calendarForEntityTypeEventStore(ENTITY_EVENT, s);
    c.title = a.title;
    const def = s.defaultCalendarForNewEvents;
    if (isNil(def)) fail('unavailable', 'no default calendar account');
    c.source = def.source;
    const err = Ref();
    check(s.saveCalendarCommitError(c, true, err), err, 'creating the calendar');
    return calendarOut(c);
  },
  calendar_delete: function (a) {
    const s = store();
    const c = calendarById(s, a.id);
    if (!a.force) {
      const cals = $.NSMutableArray.array;
      cals.addObject(c);
      const now = Date.now();
      if (Number(fetch(s, now - 1400 * DAY_MS, now + 1400 * DAY_MS, cals).count) > 0) fail('conflict', 'calendar is not empty');
    }
    const err = Ref();
    check(s.removeCalendarCommitError(c, true, err), err, 'deleting the calendar');
    return { id: a.id };
  },
};
