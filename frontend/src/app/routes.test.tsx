import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Shell } from "./Shell";
import { routeForPath, routes } from "./routes";

describe("application routes", () => {
  it.each(routes)("opens $path directly", ({ path }) => {
    expect(routeForPath(path)).toBe(path);
  });

  it("keeps a direct pet profile path", () => {
    expect(routeForPath("/tiere/momo")).toBe("/tiere/momo");
  });

  it("uses the dashboard for the root and unknown paths", () => {
    expect(routeForPath("/")).toBe("/dashboard");
    expect(routeForPath("/unbekannt")).toBe("/dashboard");
  });

  it("renders all six navigation targets with the current page marked", () => {
    const html = renderToStaticMarkup(<Shell route="/kalender" navigate={() => {}} logout={() => {}} message=""><p>Inhalt</p></Shell>);
    for (const route of routes) expect(html).toContain(route.label);
    expect(html).toContain('aria-current="page"');
  });
});
