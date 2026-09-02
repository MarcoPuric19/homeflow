import { useCallback, useEffect, useState } from "react";
import { ApiError, api, type CalendarEvent, type Expense, type ExpenseSummary, type Pet, type Reminder, type ReminderRules, type ShoppingItem, type ShoppingList } from "../api";
import { currentMonth, monthRange } from "../format";
import { Calendar, Expenses, Login, Pets, Shopping } from "../pages/LegacyPages";
import { DashboardPage, DocumentsPage } from "../pages/Placeholders";
import { Shell } from "./Shell";
import { useRoute } from "./useRoute";

export default function App() {
  const { route, navigate } = useRoute();
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [items, setItems] = useState<ShoppingItem[]>([]);
  const [shoppingLists, setShoppingLists] = useState<ShoppingList[]>([]);
  const [expenses, setExpenses] = useState<Expense[]>([]);
  const [events, setEvents] = useState<CalendarEvent[]>([]);
  const [pets, setPets] = useState<Pet[]>([]);
  const [summary, setSummary] = useState<ExpenseSummary | null>(null);
  const [reminders, setReminders] = useState<Reminder[]>([]);
  const [reminderRules, setReminderRules] = useState<ReminderRules | null>(null);
  const [month, setMonth] = useState(currentMonth());
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    try {
      const horizon = new Date(); horizon.setDate(horizon.getDate() + 366);
      const [shopping, lists, expenseList, monthSummary, calendar, petPage, reminderPage, rules] = await Promise.all([api.shopping(), api.shoppingLists(), api.expenses(month), api.summary(month), api.calendarEvents(monthRange(month)), api.pets(), api.reminders(horizon.toISOString().slice(0, 10)), api.reminderRules()]);
      setItems(shopping.items); setShoppingLists(lists.lists); setExpenses(expenseList.expenses); setSummary(monthSummary); setEvents(calendar.events); setPets(petPage.pets); setReminders(reminderPage.reminders); setReminderRules(rules); setAuthenticated(true); setMessage("");
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) setAuthenticated(false);
      else setMessage(error instanceof Error ? error.message : "Daten konnten nicht geladen werden");
    }
  }, [month]);

  useEffect(() => { void load(); }, [load]);

  async function action(task: () => Promise<unknown>, success: string) {
    setBusy(true); setMessage("");
    try { await task(); setMessage(success); await load(); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Aktion fehlgeschlagen"); }
    finally { setBusy(false); }
  }

  if (authenticated === null) return <div className="center"><div className="spinner" aria-label="HomeFlow lädt" /></div>;
  if (!authenticated) return <Login onSuccess={() => { setAuthenticated(true); void load(); }} />;

  const activePets = pets.filter((pet) => pet.active);
  const page = route === "/dashboard" ? <DashboardPage />
    : route === "/tiere" || route.startsWith("/tiere/") ? <Pets pets={pets} events={events} reminders={reminders} reminderRules={reminderRules} busy={busy} run={action} />
    : route === "/kalender" ? <Calendar events={events} pets={activePets} month={month} setMonth={setMonth} busy={busy} run={action} />
    : route === "/einkauf" ? <Shopping items={items} lists={shoppingLists} pets={activePets} busy={busy} run={action} />
    : route === "/finanzen" ? <Expenses items={expenses} summary={summary} pets={activePets} events={events} month={month} setMonth={setMonth} busy={busy} run={action} />
    : <DocumentsPage />;

  return <Shell route={route} navigate={navigate} message={message} logout={() => void action(async () => { await api.logout(); setAuthenticated(false); }, "")}>{page}</Shell>;
}
