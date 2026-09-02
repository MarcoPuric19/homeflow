import { useEffect, useState } from "react";
import { api, type VersionInfo } from "../api";
import { routes, type RoutePath } from "./routes";

// Was läuft hier eigentlich? Bis HF-0105 konnte die Anwendung das nicht
// beantworten: das Binary kannte seine Version nicht, und am 2026-09-02 liefen
// unter demselben Tag drei verschiedene Images.
//
// Angezeigt wird nur, was neu ist. Ein Hinweis, der bei jedem Start erscheint,
// wird nach dem dritten Mal weggeklickt, ohne gelesen zu werden.
const GESEHEN = "homeflow.gesehene-version";

function VersionHinweis() {
  const [info, setInfo] = useState<VersionInfo | null>(null);
  useEffect(() => {
    let abgebrochen = false;
    api.version()
      .then((antwort) => { if (!abgebrochen) setInfo(antwort); })
      // Eine fehlende Versionsauskunft darf die Anwendung nicht aufhalten.
      .catch(() => {});
    return () => { abgebrochen = true; };
  }, []);
  if (!info) return null;
  let gesehen: string | null = null;
  try { gesehen = localStorage.getItem(GESEHEN); } catch { gesehen = null; }
  if (gesehen === info.version) return null;

  const schliessen = () => {
    try { localStorage.setItem(GESEHEN, info.version); } catch { /* privates Fenster */ }
    setInfo(null);
  };
  return <section className="version-note" role="status">
    <h2>Neu in {info.version}</h2>
    {info.aktuell
      ? <ul>{info.aktuell.tickets.map((t) =>
          <li key={t.key}><b>{t.key}</b> {t.titel}{t.komponente ? ` (${t.komponente})` : ""}</li>)}</ul>
      : <p>{info.hinweis ?? "Zu dieser Version gibt es keine Notiz."}</p>}
    <button className="quiet" onClick={schliessen}>Verstanden</button>
  </section>;
}

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
        <VersionHinweis />
      {children}
      <footer>Privat · lokal gespeichert · keine Cloud</footer>
    </main>
  </div>;
}
