import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Calendar, EventRow, ExpenseRow, Pets } from "./App";
import { Expenses, Shopping, groupByDay } from "./pages/LegacyPages";
import type { CalendarEvent, Expense, Pet } from "./api";

const expense = (overrides: Partial<Expense> = {}): Expense => ({
  id: "e1", description: "Strom", plannedAmountCents: 4290, currency: "EUR",
  dueDate: "2026-08-31", status: "planned", ...overrides,
});

const render = (item: Expense) => renderToStaticMarkup(
  <ExpenseRow expense={item} busy={false} run={async () => {}} />,
);

describe("ExpenseRow", () => {
  it("offers an actual amount field prefilled with the planned amount", () => {
    const html = render(expense());
    expect(html).toContain('name="actualAmount"');
    expect(html).toContain('value="42.90"');
  });

  it("keeps the field out of the way once the expense is paid", () => {
    const html = render(expense({ status: "paid", actualAmountCents: 4290 }));
    expect(html).not.toContain('name="actualAmount"');
  });

  it("shows the planned amount next to a deviating actual amount", () => {
    const html = render(expense({ status: "paid", actualAmountCents: 5100 }));
    expect(html).toContain("51,00");
    expect(html).toContain("geplant");
  });

  it("stays quiet when actual and planned match", () => {
    const html = render(expense({ status: "paid", actualAmountCents: 4290 }));
    expect(html).not.toContain("geplant");
  });
});

describe("ExpenseRow receipts", () => {
  it("offers a receipt disclosure without loading anything up front", () => {
    const html = render(expense());
    expect(html).toContain("Belege");
    // The list is only fetched when the disclosure is opened, so a closed row
    // must not contain a download link.
    expect(html).not.toContain("/content");
  });
});

describe("ExpenseRow pet relationships", () => {
  it("offers labelled medical and appointment relationships only for pet expenses", () => {
    const pet: Pet = { id: "p1", name: "Momo", species: "Katze", approximateBirthDate: false, sex: "female", active: true, createdAt: "2026-09-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z" };
    const html = renderToStaticMarkup(<ExpenseRow expense={expense({ petId: pet.id })} pet={pet} busy={false} run={async () => {}} />);
    expect(html).toContain("Patientenakte und Termin zuordnen");
    expect(render(expense())).not.toContain("Patientenakte und Termin zuordnen");
  });
});


const event = (overrides: Partial<CalendarEvent> = {}): CalendarEvent => ({
  id: "c1", title: "Zahnarzt", startsAt: "2026-09-10T07:00:00Z", endsAt: "2026-09-10T08:00:00Z",
  allDay: false, ownerId: "u1", visibility: "shared", status: "planned", ...overrides,
});

describe("groupByDay", () => {
  it("keeps the API order and opens a group per local day", () => {
    const groups = groupByDay([
      event({ id: "a", startsAt: "2026-09-10T07:00:00Z", endsAt: "2026-09-10T08:00:00Z" }),
      event({ id: "b", startsAt: "2026-09-10T09:00:00Z", endsAt: "2026-09-10T10:00:00Z" }),
      event({ id: "c", startsAt: "2026-09-11T09:00:00Z", endsAt: "2026-09-11T10:00:00Z" }),
    ]);
    expect(groups).toHaveLength(2);
    expect(groups[0][1].map((e) => e.id)).toEqual(["a", "b"]);
    expect(groups[1][1].map((e) => e.id)).toEqual(["c"]);
  });

  it("returns nothing for an empty month", () => {
    expect(groupByDay([])).toEqual([]);
  });
});

describe("EventRow", () => {
  const renderRow = (item: CalendarEvent) =>
    renderToStaticMarkup(<EventRow event={item} busy={false} run={async () => {}} />);

  it("shows a time range for a timed event", () => {
    const html = renderRow(event());
    expect(html).toContain("–");
    expect(html).toContain("Geplant");
    expect(html).toContain("Löschen");
  });

  it("shows all-day instead of a time range", () => {
    const html = renderRow(event({ allDay: true }));
    expect(html).toContain("Ganztägig");
  });

  it("marks a private event", () => {
    expect(renderRow(event({ visibility: "private" }))).toContain("Privat");
    expect(renderRow(event())).not.toContain("Privat");
  });

  it("offers reopening instead of completing once done", () => {
    const html = renderRow(event({ status: "completed" }));
    expect(html).toContain("Erledigt");
    expect(html).toContain("Wieder öffnen");
  });
});

describe("Calendar", () => {
  const renderCalendar = (events: CalendarEvent[]) =>
    renderToStaticMarkup(<Calendar events={events} pets={[]} month="2026-09" setMonth={() => {}} busy={false} run={async () => {}} />);

  it("explains an empty month instead of showing an empty list", () => {
    const html = renderCalendar([]);
    expect(html).toContain("Nichts geplant");
  });

  it("counts only planned events in the headline", () => {
    const html = renderCalendar([event({ id: "a" }), event({ id: "b", status: "completed" })]);
    expect(html).toContain("<strong>1</strong>");
  });

  it("starts with timed inputs and a shared default", () => {
    const html = renderCalendar([]);
    expect(html).toContain('type="datetime-local"');
    expect(html).toContain("Nur für mich");
  });

  it("requires a pet and offers no generic appointment", () => {
    const html = renderCalendar([]);
    expect(html).toContain('name="petId"');
    expect(html).toContain("Tier auswählen");
    expect(html).not.toContain("Kein Tierbezug");
  });
});

describe("pet-scoped forms", () => {
  const pet: Pet = { id: "p1", name: "Momo", species: "Katze", approximateBirthDate: false, sex: "female", active: true, createdAt: "2026-09-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z" };

  it("requires an expense to select a pet", () => {
    const html = renderToStaticMarkup(<Expenses items={[]} summary={null} pets={[pet]} events={[]} month="2026-09" setMonth={() => {}} busy={false} run={async () => {}} />);
    expect(html).toContain("Tier auswählen");
    expect(html).not.toContain("Kein Tierbezug");
  });

  it("makes all pets an explicit shopping choice", () => {
    const html = renderToStaticMarkup(<Shopping items={[]} lists={[{ id: "l1", name: "Tierbedarf", visibility: "shared" }]} pets={[pet]} busy={false} run={async () => {}} />);
    expect(html).toContain("Für welche Tiere?");
    expect(html).toContain("Alle Tiere");
    expect(html).not.toContain("Für Tiere (optional)");
  });
});

describe("Pets", () => {
  const pet: Pet = { id: "p1", name: "Momo", species: "Katze", breed: "EKH", approximateBirthDate: false, sex: "female", active: true, createdAt: "2026-09-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z" };
  it("shows pet master data and reversible archiving", () => {
    const html = renderToStaticMarkup(<Pets pets={[pet]} busy={false} run={async () => {}} />);
    expect(html).toContain("Momo"); expect(html).toContain("Katze"); expect(html).toContain("Archivieren"); expect(html).toContain("Patientenakte"); expect(html).toContain("Impfungen"); expect(html).toContain("Hersteller"); expect(html).toContain("Krankheiten, kommasepariert"); expect(html).toContain("Medikamente"); expect(html).toContain("keine Diagnose- oder Behandlungsempfehlungen"); expect(html).toContain("berechnet keine Impfintervalle"); expect(html).toContain("berechnet weder Dosis noch Verbrauch");
  });
  it("explains the empty state", () => {
    expect(renderToStaticMarkup(<Pets pets={[]} busy={false} run={async () => {}} />)).toContain("Noch kein Tier angelegt");
  });
});
