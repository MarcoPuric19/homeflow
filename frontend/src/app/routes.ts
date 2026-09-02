export const routes = [
  { path: "/dashboard", label: "Dashboard", icon: "⌂" },
  { path: "/tiere", label: "Tiere", icon: "♢" },
  { path: "/kalender", label: "Kalender", icon: "□" },
  { path: "/einkauf", label: "Einkauf", icon: "✓" },
  { path: "/finanzen", label: "Finanzen", icon: "€" },
  { path: "/dokumente", label: "Dokumente", icon: "▤" },
] as const;

export type RoutePath = (typeof routes)[number]["path"] | `/tiere/${string}`;

export function routeForPath(pathname: string): RoutePath {
  if (/^\/tiere\/[^/]+$/.test(pathname)) return pathname as `/tiere/${string}`;
  const route = routes.find((candidate) => candidate.path === pathname);
  return route?.path ?? "/dashboard";
}
