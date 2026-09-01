export const euros = (cents: number) => new Intl.NumberFormat("de-DE", { style: "currency", currency: "EUR" }).format(cents / 100);
export const currentMonth = () => new Date().toISOString().slice(0, 7);

// Value for a number input, always with a dot separator as the HTML spec requires.
export const amountInputValue = (cents: number) => (cents / 100).toFixed(2);

// Parses a user supplied amount into cents without going through binary floats,
// so 1,15 stays 115 instead of 114. Returns null for anything unusable.
export function centsFromAmountInput(value: string): number | null {
  const match = /^(\d+)(?:[.,](\d{1,2}))?$/.exec(value.trim().replace(/\s/g, ""));
  if (!match) return null;
  const [, whole, fraction = ""] = match;
  const cents = Number(whole) * 100 + Number(fraction.padEnd(2, "0"));
  return Number.isSafeInteger(cents) ? cents : null;
}

// The calendar works in local time because that is what the two people using
// it think in; the API speaks instants. Both conversions live here so they can
// be tested without a browser.
export function monthRange(month: string): { from: string; to: string } {
  const [year, index] = month.split("-").map(Number);
  return {
    from: new Date(year, index - 1, 1).toISOString(),
    to: new Date(year, index, 1).toISOString(),
  };
}

// Turns the values of the form fields into instants. All-day entries carry a
// date only and cover the whole local day; timed entries carry wall-clock time
// in the local zone. Returns null when a value is missing or the end lies
// before the start, so the caller can report it instead of the API.
export function eventInstants(start: string, end: string, allDay: boolean): { startsAt: string; endsAt: string } | null {
  if (!start || !end) return null;
  const startsAt = new Date(allDay ? `${start}T00:00` : start);
  const endsAt = new Date(allDay ? `${end}T23:59` : end);
  if (Number.isNaN(startsAt.getTime()) || Number.isNaN(endsAt.getTime()) || endsAt < startsAt) return null;
  return { startsAt: startsAt.toISOString(), endsAt: endsAt.toISOString() };
}

export const eventDay = (iso: string) =>
  new Date(iso).toLocaleDateString("de-DE", { weekday: "long", day: "2-digit", month: "long" });

export const eventClock = (iso: string) =>
  new Date(iso).toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" });

// Value for a datetime-local input, in local time and without seconds.
export function localInputValue(iso: string, dateOnly = false): string {
  const value = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  const date = `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}`;
  return dateOnly ? date : `${date}T${pad(value.getHours())}:${pad(value.getMinutes())}`;
}
