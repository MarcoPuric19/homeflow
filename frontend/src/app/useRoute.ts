import { useEffect, useState } from "react";
import { routeForPath, type RoutePath } from "./routes";

export function useRoute() {
  const [route, setRoute] = useState<RoutePath>(() => routeForPath(window.location.pathname));

  useEffect(() => {
    const onPopState = () => setRoute(routeForPath(window.location.pathname));
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  function navigate(path: RoutePath) {
    if (window.location.pathname !== path) window.history.pushState({}, "", path);
    setRoute(routeForPath(path));
  }

  return { route, navigate };
}
