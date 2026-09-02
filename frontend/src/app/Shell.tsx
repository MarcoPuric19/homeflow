import { routes, type RoutePath } from "./routes";

export function Shell({ route, navigate, logout, message, children }: { route: RoutePath; navigate: (path: RoutePath) => void; logout: () => void; message: string; children: React.ReactNode }) {
  const section = route.startsWith("/tiere/") ? "/tiere" : route;
  return <div className="shell">
    <aside className="app-sidebar">
      <button className="brand brand-button" onClick={() => navigate("/dashboard")} aria-label="HomeFlow Dashboard"><span>H</span><b>HomeFlow</b></button>
      <nav className="app-nav" aria-label="Hauptnavigation">
        {routes.map((item) => <button key={item.path} className={section === item.path ? "active" : ""} aria-current={section === item.path ? "page" : undefined} onClick={() => navigate(item.path)}><span aria-hidden="true">{item.icon}</span><b>{item.label}</b></button>)}
      </nav>
      <button className="quiet logout" onClick={logout}>Abmelden</button>
    </aside>
    <main className="app-main">
      {message && <div className="notice" role="status">{message}</div>}
      {children}
      <footer>Privat · lokal gespeichert · keine Cloud</footer>
    </main>
  </div>;
}
