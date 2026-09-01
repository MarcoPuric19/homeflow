import { describe, expect, it } from "vitest";
import { amountInputValue, centsFromAmountInput, currentMonth, euros, eventClock, eventDay, eventInstants, localInputValue, monthRange } from "./format";

describe("format helpers", () => {
  it("formats cents as euros", () => expect(euros(12345)).toContain("123,45"));
  it("returns an ISO month", () => expect(currentMonth()).toMatch(/^\d{4}-\d{2}$/));
});

describe("amountInputValue", () => {
  it("renders cents with two decimals", () => expect(amountInputValue(4290)).toBe("42.90"));
  it("renders whole euros with two decimals", () => expect(amountInputValue(4200)).toBe("42.00"));
  it("renders zero", () => expect(amountInputValue(0)).toBe("0.00"));
});

describe("centsFromAmountInput", () => {
  it("accepts a dot separator", () => expect(centsFromAmountInput("42.90")).toBe(4290));
  it("accepts a comma separator", () => expect(centsFromAmountInput("42,90")).toBe(4290));
  it("accepts whole euros", () => expect(centsFromAmountInput("42")).toBe(4200));
  it("accepts a single decimal", () => expect(centsFromAmountInput("42,9")).toBe(4290));
  it("accepts zero", () => expect(centsFromAmountInput("0")).toBe(0));
  it("ignores surrounding whitespace", () => expect(centsFromAmountInput(" 42,90 ")).toBe(4290));
  it("does not lose a cent to float rounding", () => expect(centsFromAmountInput("1,15")).toBe(115));
  it("round-trips an input value", () => expect(centsFromAmountInput(amountInputValue(1999))).toBe(1999));
  it("rejects an empty value", () => expect(centsFromAmountInput("")).toBeNull());
  it("rejects more than two decimals", () => expect(centsFromAmountInput("42,901")).toBeNull());
  it("rejects negative amounts", () => expect(centsFromAmountInput("-1")).toBeNull());
  it("rejects a currency symbol", () => expect(centsFromAmountInput("42,90 €")).toBeNull());
  it("rejects letters", () => expect(centsFromAmountInput("zwölf")).toBeNull());
});

describe("monthRange", () => {
  it("spans exactly one local month", () => {
    const { from, to } = monthRange("2026-09");
    expect(new Date(from).getFullYear()).toBe(2026);
    expect(new Date(from).getMonth()).toBe(8);
    expect(new Date(from).getDate()).toBe(1);
    expect(new Date(from).getHours()).toBe(0);
    expect(new Date(to).getMonth()).toBe(9);
    expect(new Date(to).getDate()).toBe(1);
  });

  it("handles the year boundary", () => {
    const { from, to } = monthRange("2026-12");
    expect(new Date(to).getFullYear()).toBe(2027);
    expect(new Date(to).getMonth()).toBe(0);
    expect(new Date(to) > new Date(from)).toBe(true);
  });
});

describe("eventInstants", () => {
  it("keeps the local wall clock of a timed event", () => {
    const result = eventInstants("2026-09-10T09:30", "2026-09-10T10:45", false);
    expect(result).not.toBeNull();
    expect(new Date(result!.startsAt).getHours()).toBe(9);
    expect(new Date(result!.startsAt).getMinutes()).toBe(30);
    expect(new Date(result!.endsAt).getHours()).toBe(10);
  });

  it("covers the whole local day for an all-day event", () => {
    const result = eventInstants("2026-09-10", "2026-09-11", true);
    expect(result).not.toBeNull();
    const startsAt = new Date(result!.startsAt);
    const endsAt = new Date(result!.endsAt);
    expect(startsAt.getHours()).toBe(0);
    expect(startsAt.getDate()).toBe(10);
    expect(endsAt.getDate()).toBe(11);
    expect(endsAt.getHours()).toBe(23);
  });

  it("accepts an event without duration", () => {
    expect(eventInstants("2026-09-10T09:00", "2026-09-10T09:00", false)).not.toBeNull();
  });

  it("rejects an end before the start", () => {
    expect(eventInstants("2026-09-10T10:00", "2026-09-10T09:00", false)).toBeNull();
  });

  it("rejects missing and unparsable values", () => {
    expect(eventInstants("", "2026-09-10T09:00", false)).toBeNull();
    expect(eventInstants("2026-09-10T09:00", "", false)).toBeNull();
    expect(eventInstants("gestern", "morgen", false)).toBeNull();
  });
});

describe("localInputValue", () => {
  it("round-trips through eventInstants", () => {
    const instants = eventInstants("2026-09-10T09:30", "2026-09-10T10:45", false)!;
    expect(localInputValue(instants.startsAt)).toBe("2026-09-10T09:30");
    expect(localInputValue(instants.startsAt, true)).toBe("2026-09-10");
  });
});

describe("event display helpers", () => {
  it("renders a weekday and a clock time in German", () => {
    const iso = eventInstants("2026-09-10T09:05", "2026-09-10T10:00", false)!.startsAt;
    expect(eventDay(iso)).toContain("September");
    expect(eventClock(iso)).toBe("09:05");
  });
});
